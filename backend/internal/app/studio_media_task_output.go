package app

import (
	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
)

// The authenticated original receipt is the display authority, including after
// a worker restart. HTTP errors, attempt counts and billing absence prove nothing.
func (s *Service) projectStudioMediaFailures(userID string, tasks []model.Task) error {
	indexes := map[string]int{}
	ids := []string{}
	for i := range tasks {
		if tasks[i].Provider == "platform" && tasks[i].Status == model.TaskStatusFailed {
			indexes[tasks[i].ID] = i
			ids = append(ids, tasks[i].ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.repo.StudioExecutionResponses(userID, ids)
	if err != nil {
		return err
	}
	for _, row := range rows {
		var receipt platformclient.MediaRecovery
		if row.ResponseEncrypted == "" || s.decryptStudioValue(row.ResponseEncrypted, &receipt) != nil || !studioMediaRejected(receipt, "studio-"+row.TaskID) {
			continue
		}
		task := &tasks[indexes[row.TaskID]]
		if receipt.Error != nil {
			task.Stage = studioMediaRejectedStage
			task.Error, task.ExecutionDiagnosticJSON = studioMediaFailureDiagnostic(task.ID, receipt.Error, false)
			continue
		}
		task.Stage, task.Error = studioMediaRejectedStage, studioMediaRejectedMessage
		task.ExecutionDiagnosticJSON = studioMediaSubmissionDiagnostic(nil, true, task.ID)
	}
	return nil
}
