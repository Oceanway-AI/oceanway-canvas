package model

import "time"

const (
	TaskCancellationUserRequest     = "user_request"
	TaskCancellationParentCancelled = "parent_cancelled"
	TaskCancellationParentFailed    = "parent_failed"
	TaskCancellationWorkerContext   = "worker_context"
)

// TaskExecutionDiagnostic records execution facts independently of the node's
// presentation state. SafeMessage is suitable for users and model context.
type TaskExecutionDiagnostic struct {
	Category             string `json:"category,omitempty"`
	Source               string `json:"source,omitempty"`
	RecoveryAction       string `json:"recoveryAction,omitempty"`
	RecoveryAvailability string `json:"recoveryAvailability,omitempty"`
	Code                 string `json:"code"`
	Phase                string `json:"phase"`
	FieldPath            string `json:"fieldPath,omitempty"`
	TaskSubmitted        bool   `json:"taskSubmitted"`
	SubmissionOutcome    string `json:"submissionOutcome"`
	RetryClass           string `json:"retryClass"`
	SafeMessage          string `json:"safeMessage"`
	DiagnosticID         string `json:"diagnosticId"`
	WritebackOutcome     string `json:"writebackOutcome,omitempty"`
	WritebackReason      string `json:"writebackReason,omitempty"`
	WritebackNodeID      string `json:"writebackNodeId,omitempty"`
	WritebackMessage     string `json:"writebackMessage,omitempty"`
}

type TaskCancellationIntent struct {
	Source      string
	ActorID     string
	RequestedAt time.Time
	Diagnostic  TaskExecutionDiagnostic
}
