package handler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

type studioReplayRecorder struct {
	*httptest.ResponseRecorder
	onFlush func(string)
	offset  int
}

func (r *studioReplayRecorder) Flush() {
	r.ResponseRecorder.Flush()
	frame := string(r.Body.Bytes()[r.offset:])
	r.offset = r.Body.Len()
	if r.onFlush != nil {
		r.onFlush(frame)
	}
}

func TestStudioAgentHotReplay(t *testing.T) {
	const total = 1003
	for _, mode := range []string{"reconnect", "resume-zero", "initial-tail", "live-burst", "missing-row"} {
		t.Run(mode, func(t *testing.T) {
			f := newStudioFixture(t)
			user := f.user(t, "alice").ID
			key := "hot-replay"
			id := fmt.Sprintf("ag%x", sha256.Sum256([]byte(user+"\x00"+key)))[:34]
			input := fmt.Sprintf(`{"cloudAgent":{"version":1,"request":{"idempotencyKey":%q}}}`, key)
			task := model.Task{ID: id, UserID: user, Operation: "cloud_agent", Status: model.TaskStatusRunning, InputJSON: input}
			hash := fmt.Sprintf("%x", sha256.Sum256(nil))
			checkpoint := fmt.Sprintf(`{"request":{"prompt":"test","idempotencyKey":%q,"permissionMode":"read_only","logicalModelId":"model","budget":{"maxCredits":1}},
				"policy":{"compilerVersion":"frozen","systemPolicyId":"system","systemPolicyVersion":1,"mediaPolicyId":"media","mediaPolicyVersion":1,"capabilitySetVersion":"frozen","reasoningMode":"auto","profileRevision":"%[2]s","profileHash":"%[2]s","systemPolicyHash":"%[2]s","mediaPolicyHash":"%[2]s","capabilitySetHash":"%[2]s"},
				"profile":{"revision":"%[2]s","hash":"%[2]s"},"taskIds":[%[3]q],"decisions":{},"events":[]}`, key, hash, id)
			run := model.CloudAgentExecution{ID: id, UserID: user, Status: "completed", Revision: 1, CheckpointVersion: 2, EventCount: total, StateJSON: checkpoint}
			cursor := 0
			if mode == "reconnect" || mode == "missing-row" {
				cursor = 1
			}
			if mode == "live-burst" {
				run.Status, run.EventCount = "running", 0
			}
			for _, row := range []any{&task, &run} {
				if err := f.db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			addRows := func() {
				rows := make([]model.CloudAgentEventRecord, 0, total)
				for seq := 1; seq <= total; seq++ {
					if mode == "missing-row" && seq == 9 {
						continue
					}
					raw := fmt.Sprintf(`{"runId":%q,"eventId":"%s:%d","seq":%d,"type":"text","payload":{}}`, id, id, seq, seq)
					rows = append(rows, model.CloudAgentEventRecord{RunID: id, UserID: user, Sequence: seq, EventJSON: raw})
				}
				if err := f.db.CreateInBatches(rows, 100).Error; err != nil {
					t.Fatal(err)
				}
			}
			if mode != "live-burst" {
				addRows()
			}
			path := "/api/agent/runs/" + id + "/events?after=" + strconv.Itoa(cursor)
			f.request(t, "GET", path, "platform_session=bob", "", 404)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			req := httptest.NewRequestWithContext(ctx, "GET", "https://studio.example.test"+path, nil)
			req.Header.Set("Cookie", "platform_session=alice")
			if mode == "resume-zero" || mode == "reconnect" {
				req.Header.Set("Last-Event-ID", strconv.Itoa(cursor))
			}
			rec := &studioReplayRecorder{ResponseRecorder: httptest.NewRecorder()}
			if mode == "live-burst" {
				rec.onFlush = func(frame string) {
					if strings.Contains(frame, "event: run_snapshot") {
						rec.onFlush = nil
						addRows()
						if err := f.db.Model(&run).Updates(map[string]any{"status": "completed", "event_count": total, "revision": 2}).Error; err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			f.router.ServeHTTP(rec, req)
			body := rec.Body.String()
			if mode == "missing-row" {
				if rec.Code < 500 || strings.Contains(body, "run_snapshot") {
					t.Fatalf("event gap did not prevent success: %d", rec.Code)
				}
				return
			}
			next := cursor + 1
			if mode == "initial-tail" {
				next = total - repository.CloudAgentJournalWindow + 1
			}
			final := 0
			for _, line := range strings.Split(body, "\n") {
				if strings.HasPrefix(line, "id: ") {
					if line != fmt.Sprintf("id: %d", next) {
						t.Fatalf("expected seq %d, got %s", next, line)
					}
					next++
				}
				if strings.HasPrefix(line, "data: ") && strings.Contains(line, `"status":"completed"`) {
					final++
					if next != total+1 {
						t.Fatalf("terminal snapshot after %d of %d", next-1, total)
					}
				}
			}
			if rec.Code != 200 || final != 1 || ctx.Err() != nil {
				t.Fatalf("status=%d snapshots=%d last=%d", rec.Code, final, next-1)
			}
		})
	}
}
