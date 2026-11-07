package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

func (s *Service) StartStudioWorker() {
	if !s.UsesPlatformIdentity() {
		return
	}
	ctx, started := s.backgroundWorkers().Start()
	if !started {
		return
	}
	s.taskWorker().start(ctx)
	s.runWorkerLoop(func(ctx context.Context) {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			s.releaseCompletedStudioExecutions(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
}

func (s *Service) processStudioTask(ctx context.Context, task *model.Task) error {
	if task.Provider != "platform" {
		return s.failStudioTask(task, "此任务尚未接通平台执行")
	}
	execution, err := s.repo.StudioExecution(task.UserID, task.ID)
	if err != nil {
		return err
	}
	var envelope studioExecutionEnvelope
	if err := s.decryptStudioValue(execution.EnvelopeEncrypted, &envelope); err != nil {
		return err
	}
	if envelope.Task.ID != task.ID || envelope.Task.UserID != task.UserID {
		return errors.New("平台执行记录归属无效")
	}
	if !strings.HasPrefix(execution.AuthorizationEncrypted, encryptedSettingPrefix) {
		return errors.New("平台执行授权密文无效")
	}
	authorization, err := s.decryptSettingSecret(execution.AuthorizationEncrypted)
	if err != nil {
		return errors.New("平台执行授权无法解密")
	}
	exec := platformclient.ExecutionContext{Authorization: authorization}
	if envelope.Selection.Kind != "text" {
		return s.processStudioMedia(ctx, task, execution, envelope, exec)
	}
	var raw []byte
	if execution.ResponseEncrypted != "" {
		if err := s.decryptStudioValue(execution.ResponseEncrypted, &raw); err != nil {
			return err
		}
	} else {
		first, err := s.repo.SubmitStudioExecution(task)
		if err != nil {
			return err
		}
		if first {
			response, err := s.studioPlatform.Client.SubmitText(ctx, exec, envelope.Selection.TextProtocol, envelope.RequestID, envelope.Body)
			if err != nil || response.StatusCode == 202 {
				return s.deferStudioTask(task)
			}
			raw = response.Body
		} else {
			recovery, err := s.studioPlatform.Client.RecoverText(ctx, exec, envelope.RequestID)
			if err != nil {
				return s.deferStudioTask(task)
			}
			if recovery.Data.RequestID != envelope.RequestID {
				return s.deferStudioTask(task)
			}
			if !recovery.Data.Ready {
				if recovery.Data.Status == "FAILURE" || recovery.Data.Status == "failed" {
					return s.failStudioTask(task, "平台原请求执行失败，请核对任务记录")
				}
				return s.deferStudioTask(task)
			}
			fingerprint := fmt.Sprintf("%x", sha256.Sum256(append([]byte(envelope.Selection.Protocol+"\x00"), envelope.Body...)))
			if recovery.Data.Protocol != envelope.Selection.Protocol || recovery.Data.Fingerprint != fingerprint {
				return s.deferStudioTask(task)
			}
			raw = recovery.Data.Response
		}
		encrypted, err := s.encryptStudioValue(raw)
		if err != nil {
			return err
		}
		if err := s.repo.SaveStudioExecutionResponse(task, encrypted); err != nil {
			return err
		}
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return s.failStudioTask(task, "平台原响应无法解析，已保留原任务，请核对结果")
	}
	result, err := parseAgentToolPayload(payload, studioTextWire(envelope.Selection.TextProtocol))
	if err != nil || stringValue(result["text"]) == "" && len(interfaceSlice(result["toolCalls"])) == 0 {
		return s.failStudioTask(task, "平台原响应没有可用内容，已保留原任务，请核对结果")
	}
	return s.completeStudioTask(ctx, task, result)
}

func (s *Service) completeStudioTask(ctx context.Context, task *model.Task, result map[string]any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	now := time.Now()
	task.Status, task.Stage, task.Progress, task.ResultJSON, task.CompletedAt, task.Error = model.TaskStatusSucceeded, "生成完成", 100, string(encoded), &now, ""
	task.NextPollAt = nil
	err = s.repo.SaveTaskCompletionWithRegistration(task, model.TaskStatusRunning, nil, func(repo *repository.Repository) error { return repo.CompleteStudioExecution(task.ID) })
	if err != nil {
		return err
	}
	s.releaseCompletedStudioExecutions(ctx)
	return nil
}

func (s *Service) deferStudioTask(task *model.Task) error {
	return s.repo.DeferRunningTaskForProviderPoll(task.ID, task.LeaseOwner, "正在查询平台原请求，请勿重复生成", 5*time.Second)
}

func (s *Service) failStudioTask(task *model.Task, message string) error {
	return s.repo.Transaction(func(repo *repository.Repository) error {
		changed, err := repo.UpdateTaskTerminalState(task.ID, task.LeaseOwner, model.TaskStatusRunning, model.TaskStatusFailed, "原任务待核对", message, time.Now())
		if err != nil {
			return err
		}
		if !changed {
			return repository.ErrTaskStateConflict
		}
		return repo.CompleteStudioExecution(task.ID)
	})
}

func (s *Service) releaseCompletedStudioExecutions(ctx context.Context) {
	rows, err := s.repo.StudioExecutionsToRelease()
	if err != nil {
		return
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		if !strings.HasPrefix(row.AuthorizationEncrypted, encryptedSettingPrefix) {
			continue
		}
		auth, err := s.decryptSettingSecret(row.AuthorizationEncrypted)
		if err != nil {
			continue
		}
		call, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, err = s.studioPlatform.Client.ReleaseExecution(call, auth)
		cancel()
		if err == nil {
			_ = s.repo.ReleaseStudioExecution(row.TaskID)
		}
	}
}
