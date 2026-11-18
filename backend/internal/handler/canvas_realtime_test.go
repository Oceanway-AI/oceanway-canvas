package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"yingce/backend/internal/model"
	"yingce/backend/internal/service"
)

func TestCanvasRealtimeHTTPAuthorizationBroadcastAndInvalidation(t *testing.T) {
	f := newStudioFixture(t)
	db := f.db
	for _, row := range []any{
		&model.CanvasProject{ID: "canvas", UserID: "101", Revision: 1, Title: "Canvas", PayloadJSON: `{"id":"canvas","title":"Canvas","nodes":[],"connections":[]}`},
		&model.CanvasProject{ID: "foreign", UserID: "other", Revision: 1, PayloadJSON: `{}`},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := f.svc
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Host = "studio.example.test"
		f.router.ServeHTTP(w, r)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/canvas-projects/"
	options := &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": {"platform_session=alice"}}}
	for _, item := range []struct {
		id      string
		options *websocket.DialOptions
		status  int
	}{{"canvas", nil, 401}, {"foreign", options, 404}} {
		conn, response, err := websocket.Dial(ctx, endpoint+item.id+"/realtime", item.options)
		if conn != nil {
			conn.CloseNow()
		}
		if err == nil || response == nil || response.StatusCode != item.status {
			t.Fatalf("authorization: status=%v err=%v", response, err)
		}
	}
	read := func(conn *websocket.Conn, match func(map[string]any) bool) map[string]any {
		t.Helper()
		for {
			var event map[string]any
			if err := wsjson.Read(ctx, conn, &event); err != nil {
				t.Fatal(err)
			}
			if match(event) {
				return event
			}
		}
	}
	a, _, err := websocket.Dial(ctx, endpoint+"canvas/realtime", options)
	if err != nil {
		t.Fatal(err)
	}
	defer a.CloseNow()
	welcomeA := read(a, func(e map[string]any) bool { return e["type"] == "welcome" })
	b, _, err := websocket.Dial(ctx, endpoint+"canvas/realtime", options)
	if err != nil {
		t.Fatal(err)
	}
	defer b.CloseNow()
	welcomeB := read(b, func(e map[string]any) bool { return e["type"] == "welcome" })
	if welcomeA["selfId"] == welcomeB["selfId"] || len(welcomeB["peers"].([]any)) != 2 {
		t.Fatal("windows share an identity")
	}
	if err := wsjson.Write(ctx, a, map[string]any{"type": "presence", "cursor": map[string]any{"x": 25, "y": 45}, "drag": nil}); err != nil {
		t.Fatal(err)
	}
	read(b, func(e map[string]any) bool {
		peer, _ := e["peer"].(map[string]any)
		return peer != nil && peer["id"] == welcomeA["selfId"] && peer["cursor"] != nil
	})
	var project model.CanvasProject
	if err := db.First(&project, "id = ?", "canvas").Error; err != nil || project.Revision != 1 {
		t.Fatalf("presence persisted: %+v err=%v", project, err)
	}
	_, err = svc.ChangeUserCanvasEditLease("101", "canvas", service.CanvasEditLeaseRequest{Action: "acquire", CanvasEditProof: model.CanvasEditProof{NodeID: "node", SessionID: "window"}})
	if err != nil {
		t.Fatal(err)
	}
	read(b, func(e map[string]any) bool { return e["type"] == "invalidate" })
	_ = a.CloseNow()
	read(b, func(e map[string]any) bool { return e["type"] == "leave" && e["peerId"] == welcomeA["selfId"] })
}
