package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStudioAgentPolicyUsesPlatformBudgetAuthority(t *testing.T) {
	for _, platform := range []bool{true, false} {
		req := agentTestRequest()
		if platform {
			req.ChannelID = StudioPlatformChannelID
			req.Budget.MaxCredits = 0
		}
		text, _, err := compileCloudAgentPolicies(req, "Studio", nil, "", cloudAgentProfileSnapshot{})
		if err != nil {
			t.Fatal(err)
		}
		_, body, ok := strings.Cut(text, "本轮执行上下文：\n")
		if !ok {
			t.Fatal("missing execution facts")
		}
		var facts struct {
			Budget map[string]any `json:"budget"`
		}
		if err := json.Unmarshal([]byte(body), &facts); err != nil {
			t.Fatal(err)
		}
		_, localCap := facts.Budget["maxCredits"]
		if platform && (localCap || facts.Budget["authority"] != "platform") {
			t.Fatalf("Platform projected local budget: %v", facts.Budget)
		}
		if !platform && (!localCap || facts.Budget["maxCredits"] != req.Budget.MaxCredits) {
			t.Fatal("native local cap lost")
		}
		checkpoint := cloudAgentFallbackCheckpoint(&cloudAgentRuntime{Request: req})
		constraints := strings.Join(checkpoint.Constraints, "\n")
		if platform && (strings.Contains(constraints, "0.0000 credits") || !strings.Contains(constraints, "Platform")) {
			t.Fatalf("recovery lost platform budget: %s", constraints)
		}
		if !platform && !strings.Contains(constraints, "credits") {
			t.Fatal("native local recovery lost cap")
		}
	}
}
