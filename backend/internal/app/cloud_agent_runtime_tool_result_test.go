package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

func TestCloudAgentOversizedReadPersistsFailureAndReplays(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.Create(&model.CanvasProject{ID: "agent-canvas", UserID: "user", PayloadJSON: `{"nodes":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	view, err := s.CreateCloudAgentRun("user", agentTestRequest(), "")
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.repo.CloudAgent("user", view.ID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := cloudAgentDecode(run)
	if err != nil {
		t.Fatal(err)
	}
	call := cloudAgentCall{ID: "oversized-read"}
	call.Function.Name, call.Function.Arguments = "model_list", `{}`
	state.Calls = []cloudAgentCall{call}
	state.CallIndex = 0
	state.Canonical.Messages = append(state.Canonical.Messages, map[string]any{"role": "assistant", "content": "", "tool_calls": state.Calls})
	cloudAgentToolResult(run.ID, &state, call, map[string]any{"models": strings.Repeat("模型", 500000)}, nil)
	if err := s.repo.MutateCloudAgent("user", run.ID, run.Revision, func(current *model.CloudAgentExecution, _ *repository.Repository) error {
		return cloudAgentSave(current, &state)
	}); err != nil {
		t.Fatalf("oversized read lost its durable failure: %v", err)
	}
	result, err := s.executeCloudAgentRuntimeTool(t.Context(), "user", run.ID, call)
	if err != nil {
		t.Fatal(err)
	}
	payload := result.(map[string]any)
	var detail map[string]any
	if json.Unmarshal([]byte(payload["content"].(string)), &detail) != nil || payload["isError"] != true || detail["reason"] != "tool_result_too_large" || detail["retryable"] != false {
		t.Fatalf("bridge did not replay the bounded failure: %#v", payload)
	}
	last := state.Events[len(state.Events)-1]
	if last.Type != "tool_failed" || last.Payload["callId"] != call.ID || state.CallIndex != 1 {
		t.Fatal("tool failure did not advance exactly one call")
	}
}

// 线上复现：recall_lessons 经运行时工具桥执行后，交回模型的 tool 消息是 "null"，
// 上游拿到空结果后返回空内容，整轮失败。任何工具结果都必须是非空的 JSON 文本。
func TestCloudAgentRuntimeToolResultIsNeverNull(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.Create(&model.CanvasProject{ID: "agent-canvas", UserID: "user", PayloadJSON: `{"nodes":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	run, err := s.CreateCloudAgentRun("user", agentTestRequest(), "")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, args string }{
		{"recall_lessons", `{"topic":"用户称呼偏好"}`},
		{"recall_lessons", `{}`},
		{"canvas_list_node_types", `{}`},
		{"canvas_get_state", `{}`},
	}
	calls := make([]cloudAgentCall, 0, len(cases))
	for index, tc := range cases {
		var call cloudAgentCall
		call.ID = fmt.Sprintf("tool-result-%d", index)
		call.Function.Name = tc.name
		call.Function.Arguments = tc.args
		calls = append(calls, call)
	}

	// executeCloudAgentRuntimeTool is intentionally strict: a bridge result is
	// accepted only when the model declaration has already been checkpointed.
	// Build that same durable boundary here instead of bypassing the contract by
	// calling the executor with an arbitrary call.
	persisted, err := s.repo.CloudAgent("user", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := cloudAgentDecode(persisted)
	if err != nil {
		t.Fatal(err)
	}
	state.Calls = calls
	state.CallIndex = 0
	state.Canonical.Messages = append(state.Canonical.Messages, map[string]any{
		"role": "assistant", "content": "", "tool_calls": calls,
	})
	if err := s.repo.MutateCloudAgent("user", run.ID, persisted.Revision, func(current *model.CloudAgentExecution, _ *repository.Repository) error {
		return cloudAgentSave(current, &state)
	}); err != nil {
		t.Fatalf("persist model tool calls: %v", err)
	}

	for index, tc := range cases {
		call := calls[index]
		result, err := s.executeCloudAgentRuntimeTool(context.Background(), "user", run.ID, call)
		if err != nil {
			t.Fatalf("%s(%s) bridge error: %v", tc.name, tc.args, err)
		}
		payload, _ := result.(map[string]any)
		content, _ := payload["content"].(string)
		if strings.TrimSpace(content) == "" || content == "null" {
			t.Fatalf("%s(%s) returned empty tool content: %#v", tc.name, tc.args, result)
		}

		// The bridge can retry after losing its response. The second request must
		// replay the committed tool message, not execute the next model call.
		replayed, err := s.executeCloudAgentRuntimeTool(context.Background(), "user", run.ID, call)
		if err != nil {
			t.Fatalf("%s(%s) replay error: %v", tc.name, tc.args, err)
		}
		replayedPayload, _ := replayed.(map[string]any)
		if replayedPayload["content"] != content {
			t.Fatalf("%s(%s) replay changed committed content: first=%q replay=%#v", tc.name, tc.args, content, replayed)
		}
	}
}
