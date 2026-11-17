package app

import (
	"encoding/json"
	"strings"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
)

// Native usage comes from the retained original response. Pi expects disjoint
// token buckets, while OpenAI includes cached reads in input tokens.
func studioTextUsage(raw []byte, protocol platformclient.TextProtocol) map[string]int64 {
	var response struct {
		Usage *struct {
			Input        *int64 `json:"input_tokens"`
			Output       *int64 `json:"output_tokens"`
			Prompt       *int64 `json:"prompt_tokens"`
			Completion   *int64 `json:"completion_tokens"`
			CacheRead    int64  `json:"cache_read_input_tokens"`
			CacheWrite   int64  `json:"cache_creation_input_tokens"`
			InputDetails struct {
				Cached int64 `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			PromptDetails struct {
				Cached int64 `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Usage == nil {
		return nil
	}
	u := response.Usage
	input, output, cached, written := u.Input, u.Output, u.CacheRead, u.CacheWrite
	if protocol == platformclient.ChatCompletions {
		input, output, cached, written = u.Prompt, u.Completion, u.PromptDetails.Cached, 0
	} else if protocol == platformclient.Responses {
		cached, written = u.InputDetails.Cached, 0
	}
	if input == nil || output == nil || *input < 0 || *output < 0 || cached < 0 || written < 0 {
		return nil
	}
	uncached := *input
	if protocol != platformclient.Messages {
		uncached -= cached
	}
	if uncached < 0 {
		return nil
	}
	return map[string]int64{"input": uncached, "output": *output, "cacheRead": cached, "cacheWrite": written, "totalTokens": uncached + *output + cached + written}
}

func (s *Service) cloudAgentPiTaskContextOverflow(taskID string) bool {
	if s.UsesPlatformIdentity() {
		task, err := s.repo.Task(taskID)
		if err != nil || task.Status != model.TaskStatusFailed {
			return false
		}
		diagnostic := taskExecutionDiagnostic(task)
		if diagnostic == nil || diagnostic.SubmissionOutcome != "rejected" {
			return false
		}
		return cloudAgentContextOverflowCode(diagnostic.Code)
	}
	call, err := s.repo.LatestAPICallForTask(taskID)
	if err != nil || call.Status != model.ApiCallStatusFailed {
		return false
	}
	if cloudAgentContextOverflowCode(call.ErrorCode) {
		return true
	}
	for _, marker := range []string{"exceeds the context window", "maximum context length", "context length exceeded", "prompt is too long", "prompt too long", "input is too long", "input token count exceeds"} {
		if strings.Contains(strings.ToLower(call.Error), marker) {
			return true
		}
	}
	return false
}

func cloudAgentContextOverflowCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "context_length_exceeded", "context_window_exceeded", "prompt_too_long", "input_too_long":
		return true
	}
	return false
}
