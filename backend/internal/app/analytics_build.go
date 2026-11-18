package app

import (
	"strings"
)

func normalizeCapability(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "text", "image", "video", "audio":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func capabilityFromTaskType(taskType string) string {
	value := strings.ToLower(taskType)
	for _, capability := range []string{"video", "image", "audio", "text"} {
		if strings.Contains(value, capability) {
			return capability
		}
	}
	if strings.Contains(value, "storyboard") || strings.Contains(value, "agent") {
		return "text"
	}
	return ""
}
