package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"yingce/backend/internal/model"
)

func TestAgentResponsesToolFollowupAfterRestore(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	for _, shape := range []string{"assistant_tool_calls", "function_call"} {
		t.Run(shape, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				var body struct {
					Input []map[string]any `json:"input"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if requests == 1 {
					_, _ = w.Write([]byte(`{"output":[{"type":"function_call","id":"fc_fixture","call_id":"call_fixture","name":"canvas_get_state","arguments":"{}","status":"completed"}]}`))
					return
				}
				calls := 0
				for _, item := range body.Input {
					if item["type"] != "function_call" {
						continue
					}
					calls++
					for key := range item {
						switch key {
						case "type", "id", "call_id", "name", "arguments":
						default:
							t.Errorf("unsupported function_call property %q", key)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
					}
					if item["id"] != "fc_fixture" || item["call_id"] != "call_fixture" || item["name"] != "canvas_get_state" || item["arguments"] != "{}" {
						t.Errorf("tool call changed after restore: %#v", item)
					}
				}
				last := body.Input[len(body.Input)-1]
				if calls != 1 || last["type"] != "function_call_output" || last["call_id"] != "call_fixture" || last["output"] != `{"nodes":[]}` {
					t.Errorf("tool output pairing = %#v, calls = %d", last, calls)
				}
				_, _ = w.Write([]byte(`{"output_text":"complete","output":[]}`))
			}))
			defer server.Close()
			config := providerConfig{BaseURL: server.URL, APIKey: "fixture", Model: "fixture", InterfaceType: string(model.ChannelInterfaceOpenAIResponse)}
			request := canonicalAgentRequest{Messages: []map[string]any{{"role": "user", "content": "read canvas"}}, ToolChoice: "auto"}
			first, err := runAgentToolTask(context.Background(), canvasGenerationInput{Config: config, AgentRequests: &agentToolRequests{Canonical: &request}})
			if err != nil {
				t.Fatal(err)
			}
			assistant := map[string]any{"role": "assistant", "content": "", "tool_calls": first["toolCalls"]}
			if shape == "function_call" {
				assistant = map[string]any{"type": "function_call", "id": "fc_fixture", "call_id": "call_fixture", "name": "canvas_get_state", "arguments": "{}", "item_reference": "fc_fixture"}
			}
			request.Messages = append(request.Messages, assistant, map[string]any{"role": "tool", "tool_call_id": "call_fixture", "content": `{"nodes":[]}`})
			raw, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			var restored canonicalAgentRequest
			if err := json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			result, err := runAgentToolTask(context.Background(), canvasGenerationInput{Config: config, AgentRequests: &agentToolRequests{Canonical: &restored}})
			if err != nil {
				t.Fatal(err)
			}
			if result["text"] != "complete" || requests != 2 {
				t.Fatalf("result = %#v, requests = %d", result, requests)
			}
		})
	}
}
