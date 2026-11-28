package app

import (
	"testing"

	"yingce/backend/internal/model"
)

func TestStudioNativeTextDoesNotInventUnknownFundingReceipt(t *testing.T) {
	task := model.Task{Provider: "platform", Type: "canvas_text", Status: model.TaskStatusSucceeded, ResultJSON: `{"text":"system verified"}`}
	if got := studioTaskFunds(task); got != nil {
		t.Fatalf("native text has no media funding receipt: %+v", got)
	}
	task.ResultJSON = `{"fundsState":"settled","settledPoints":"0.0624"}`
	if got := studioTaskFunds(task); got == nil || got.SettledPoints != "0.0624" {
		t.Fatalf("an original retained receipt must still be readable: %+v", got)
	}
}
