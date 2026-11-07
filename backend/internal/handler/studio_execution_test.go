package handler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

func TestStudioTextExecutionRecoversOriginalWithoutResubmission(t *testing.T) {
	for _, wire := range []struct{ published, path, result string }{
		{"openai-chat", "chat/completions", `{"choices":[{"message":{"content":"recovered text","tool_calls":[{"id":"call_1","type":"function","function":{"name":"inspect_canvas","arguments":"{}"}}]}}]}`},
		{"openai-responses", "responses", `{"output":[{"type":"message","content":[{"type":"output_text","text":"recovered text"}]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"inspect_canvas","arguments":"{}"}]}`},
		{"anthropic-messages", "messages", `{"content":[{"type":"text","text":"recovered text"},{"type":"tool_use","id":"call_1","name":"inspect_canvas","input":{}}]}`},
	} {
		t.Run(wire.path, func(t *testing.T) {
			f := newStudioFixture(t)
			f.catalog = `{"success":true,"data":{"items":[{"model_name":"exact-model","model_type":"text","invocation_status":"published_text","supported_protocols":["` + wire.published + `"],"price":{"status":"available","version":"p1"}}]}}`
			issues, submits, recovers := 0, 0, 0
			requestID := ""
			fingerprint := ""
			f.execution = func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("X-Platform-Product-Key") != "fixture.secret" {
					t.Error("missing product authority")
				}
				if r.URL.Path == "/api/platform/v1/execution-authorizations" {
					issues++
					if r.Header.Get("Cookie") != "platform_session=alice" {
						t.Error("issue missing current account")
					}
					var in map[string]string
					_ = json.NewDecoder(r.Body).Decode(&in)
					_ = json.NewEncoder(w).Encode(map[string]any{"authorization_id": "exa_private_fixture", "product": "studio", "task_reference": in["task_reference"], "expires_at": time.Now().Add(time.Hour).Unix()})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/release") {
					w.WriteHeader(204)
					return
				}
				if r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" || r.Header.Get("X-Platform-Execution-Authorization") != "exa_private_fixture" {
					t.Error("worker authority mixed or missing")
				}
				if r.Method == "POST" {
					submits++
					if r.URL.Path != "/internal/relay/"+wire.path {
						t.Errorf("wrong wire: %s", r.URL.Path)
					}
					requestID = r.Header.Get("X-Platform-Text-Request-Id")
					body, _ := io.ReadAll(r.Body)
					fingerprint = fmt.Sprintf("%x", sha256.Sum256(append([]byte(wire.published+"\x00"), body...)))
					if !strings.Contains(string(body), `"model":"exact-model"`) || strings.Contains(string(body), `"stream":true`) || requestID == "" {
						t.Error("native body/ID invalid")
					}
					w.WriteHeader(503)
					return
				}
				recovers++
				if r.URL.Path != "/internal/relay/text-requests/"+requestID {
					t.Error("recovery changed original")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"version": "oceanway.text-request.v1", "request_id": requestID, "status": "ready", "ready": true, "protocol": wire.published, "fingerprint": fingerprint, "response": json.RawMessage(wire.result)}})
			}
			body := `{"commandId":"intent-1","type":"canvas_text","operation":"text","prompt":"hello","model":"exact-model","input":{"mode":"text","config":{"channelId":"oceanway-platform","model":"exact-model"}}}`
			w := f.request(t, "POST", "/api/tasks", "platform_session=alice", body, 200)
			var created struct {
				Data model.Task `json:"data"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &created)
			id := created.Data.ID
			if id == "" {
				t.Fatal("missing task")
			}
			f.request(t, "POST", "/api/tasks", "platform_session=alice", body, 200)
			f.request(t, "POST", "/api/tasks", "platform_session=alice", strings.Replace(body, "hello", "changed", 1), 409)
			if issues != 1 {
				t.Fatal("duplicate created authorization")
			}
			var private model.StudioExecution
			if err := f.db.First(&private, "task_id = ?", id).Error; err != nil {
				t.Fatal(err)
			}
			if strings.Contains(private.EnvelopeEncrypted, "hello") || strings.Contains(private.AuthorizationEncrypted, "exa_private") || private.AuthorizationEncrypted == "" {
				t.Fatal("execution not encrypted")
			}
			if err := f.svc.ProcessNextTask(); err != nil {
				t.Fatal(err)
			}
			var pending model.Task
			f.db.First(&pending, "id = ?", id)
			if pending.Status != model.TaskStatusRunning {
				t.Fatalf("unknown request must stay recoverable: %s", pending.Status)
			}
			f.db.Model(&model.Task{}).Where("id = ?", id).Update("next_poll_at", time.Now().Add(-time.Second))
			if err := f.svc.ProcessNextTask(); err != nil {
				t.Fatal(err)
			}
			w = f.request(t, "GET", "/api/tasks/"+id, "platform_session=alice", "", 200)
			if !strings.Contains(w.Body.String(), "recovered text") || !strings.Contains(w.Body.String(), "inspect_canvas") {
				t.Fatalf("native result/tools lost: %s; submits=%d recovers=%d", w.Body.String(), submits, recovers)
			}
			if strings.Contains(w.Body.String(), "exa_private") || strings.Contains(w.Body.String(), "fixture.secret") {
				t.Fatal("execution authority exposed")
			}
			f.request(t, "GET", "/api/tasks/"+id, "platform_session=bob", "", 404)
			if submits != 1 || recovers != 1 {
				t.Fatalf("submits=%d recovers=%d", submits, recovers)
			}
			f.request(t, "GET", "/api/tasks/by-command/intent-1", "platform_session=alice", "", 200)
			f.request(t, "GET", "/api/tasks/by-command/intent-1", "platform_session=bob", "", 404)
			var count int64
			f.db.Model(&model.BillingOrder{}).Count(&count)
			if count != 0 {
				t.Fatal("created local billing order")
			}
		})
	}
}

func TestStudioUnknownOriginalIsNeverResentAndAuthorizationCanResume(t *testing.T) {
	f := newStudioFixture(t)
	f.catalog = `{"success":true,"data":{"items":[{"model_name":"m","model_type":"text","invocation_status":"published_text","supported_protocols":["openai-chat"],"price":{"status":"available","version":"p1"}}]}}`
	issues, submits, reads := 0, 0, 0
	taskReference := ""
	f.execution = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/platform/v1/execution-authorizations" {
			issues++
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			if taskReference != "" && in["task_reference"] != taskReference {
				t.Fatal("changed task during authorization recovery")
			}
			taskReference = in["task_reference"]
			if issues == 1 {
				w.WriteHeader(503)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"authorization_id": "exa_original", "product": "studio", "task_reference": taskReference, "expires_at": time.Now().Add(time.Hour).Unix()})
			return
		}
		if r.Method == "POST" {
			submits++
			w.WriteHeader(503)
			return
		}
		reads++
		requestID := strings.TrimPrefix(r.URL.Path, "/internal/relay/text-requests/")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"request_id": requestID, "status": "not_found", "ready": false}})
	}
	body := `{"commandId":"unknown-1","type":"canvas_text","operation":"text","prompt":"hello","model":"m","input":{"config":{"channelId":"oceanway-platform","model":"m"}}}`
	f.request(t, "POST", "/api/tasks", "platform_session=alice", body, 503)
	f.request(t, "POST", "/api/tasks", "platform_session=alice", body, 200)
	for i := 0; i < 3; i++ {
		if err := f.svc.ProcessNextTask(); err != nil {
			t.Fatal(err)
		}
		f.db.Model(&model.Task{}).Where("id = ?", taskReference).Update("next_poll_at", time.Now().Add(-time.Second))
	}
	if submits != 1 || reads != 2 {
		t.Fatalf("unknown caused repeat: submits=%d reads=%d", submits, reads)
	}
	f.request(t, "POST", "/api/tasks", "platform_session=alice", strings.Replace(body, `"commandId":"unknown-1"`, `"commandId":"unsafe","provider":"external"`, 1), 400)
	f.request(t, "POST", "/api/tasks", "platform_session=alice", strings.Replace(body, `"channelId":"oceanway-platform"`, `"channelId":"oceanway-platform","apiKey":"forbidden"`, 1), 409)
}
