package app

import (
	"encoding/json"
	"testing"
	"time"

	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

func TestCloudAgentTaskDiagnosticProjectsDurableExecutionFacts(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	diagnostic := model.TaskExecutionDiagnostic{
		Code: "invalid_duration", Phase: "preparation", FieldPath: "generationSpec.options.durationSeconds",
		TaskSubmitted: false, SubmissionOutcome: "not_submitted", RetryClass: "correct_input",
		SafeMessage: "视频时长不受支持", DiagnosticID: "diag-1",
	}
	rawDiagnostic, _ := json.Marshal(diagnostic)
	task := &model.Task{
		ID: "task-facts", UserID: "101", Provider: "platform", ResultJSON: `{"fundsState":"released"}`, ProjectID: "canvas", Type: "canvas_video", Status: model.TaskStatusFailed,
		Operation: "cloud_agent_generate_media", AgentRunID: "run-1", GenerationID: "generation-1", ApprovalID: "approval-1",
		BillingOrderID: "order-facts", AuthorizedChargeMicrocredits: 700, CancellationSource: model.TaskCancellationUserRequest,
		CancellationRequestedAt: &now, InputJSON: `{"metadata":{"source":"cloud_agent","nodeId":"video-node"}}`,
		ExecutionDiagnosticJSON: string(rawDiagnostic), CreatedAt: now,
	}
	facts := cloudAgentTaskDiagnostic(nil, task)
	for key, want := range map[string]any{
		"taskId": task.ID, "taskStatus": string(model.TaskStatusFailed), "agentRunId": "run-1", "generationId": "generation-1",
		"approvalId": "approval-1", "nodeId": "video-node", "taskSubmitted": false, "submissionOutcome": "not_submitted",
		"phase": "preparation", "diagnosticCode": "invalid_duration", "diagnosticId": "diag-1",
		"fieldPath": "generationSpec.options.durationSeconds", "retryClass": "correct_input", "error": "视频时长不受支持",
		"cancellationSource": model.TaskCancellationUserRequest,
	} {
		if got := facts[key]; got != want {
			t.Fatalf("%s = %#v, want %#v; facts=%#v", key, got, want, facts)
		}
	}
	billing, ok := facts["platformBilling"].(map[string]any)
	if !ok || billing["fundsState"] != "released" || billing["stateAvailable"] != true || facts["billing"] != nil {
		t.Fatalf("Platform funds = %#v", facts)
	}
	receipt, ok := facts["submissionReceipt"].(map[string]any)
	if !ok || receipt["taskId"] != task.ID || receipt["generationId"] != task.GenerationID {
		t.Fatalf("submission receipt = %#v", facts["submissionReceipt"])
	}
}

func TestRecordTaskDiagnosticDoesNotClaimUnsubmittedRequest(t *testing.T) {
	task := &model.Task{ID: "task", Error: "参数无效"}
	recordTaskDiagnostic(task, "preparation", "preparation_failed", "not_submitted", "correct_input", nil)
	if task.Diagnostic == nil || task.Diagnostic.TaskSubmitted {
		t.Fatalf("diagnostic = %#v", task.Diagnostic)
	}
	recordTaskDiagnostic(task, "execution", "provider_execution_failed", "accepted", "new_generation_authorization", nil)
	if task.Diagnostic == nil || !task.Diagnostic.TaskSubmitted {
		t.Fatalf("diagnostic = %#v", task.Diagnostic)
	}
}

func TestCloudAgentTaskDiagnosticDoesNotReadLocalBilling(t *testing.T) {
	// A repository without a connection exposes any accidental local ledger read.
	task := &model.Task{ID: "task", UserID: "101", Status: model.TaskStatusQueued, BillingOrderID: "old-order", AuthorizedChargeMicrocredits: 9}
	facts := cloudAgentTaskDiagnostic(&repository.Repository{}, task)
	if facts["billing"] != nil || facts["platformBilling"] != nil {
		t.Fatalf("local billing state exposed: %#v", facts)
	}
}

func TestCloudAgentTaskDiagnosticPreservesPlatformSettlement(t *testing.T) {
	task := &model.Task{
		ID: "task-settled", UserID: "101", Provider: "platform", Status: model.TaskStatusSucceeded,
		ResultJSON: `{"fundsState":"settled","settledPoints":"0.100000000000000001"}`,
	}
	facts := cloudAgentTaskDiagnostic(nil, task)
	billing, ok := facts["platformBilling"].(map[string]any)
	if !ok || billing["fundsState"] != "settled" || billing["settledPoints"] != "0.100000000000000001" || facts["billing"] != nil {
		t.Fatalf("Platform settlement = %#v", facts)
	}
}
