package handler

import (
	"encoding/json"
	"testing"

	"yingce/backend/internal/model"
)

func TestStudioTaskFundsReadsOriginalPlatformReceipt(t *testing.T) {
	f := newStudioFixture(t)
	for _, tc := range []struct{ id, result, state, points string }{
		{"precise", `{"fundsState":"settled","settledPoints":"9007199254740993.123456789012345678"}`, "settled", "9007199254740993.123456789012345678"},
		{"zero", `{"fundsState":"settled","settledPoints":"0"}`, "settled", "0"},
		{"no-amount", `{"fundsState":"settled"}`, "settled", ""},
		{"unknown", `{}`, "unknown", ""},
	} {
		task := model.Task{ID: tc.id, UserID: "101", Provider: "platform", Type: "canvas_image", Status: model.TaskStatusSucceeded, ResultJSON: tc.result, BillingOrderID: "retired-order"}
		if err := f.db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
		w := f.request(t, "GET", "/api/tasks/"+tc.id, "platform_session=alice", "", 200)
		var body struct{ Data model.Task }
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		funds := body.Data.PlatformBilling
		if funds == nil || funds.FundsState != tc.state || funds.SettledPoints != tc.points || funds.StateAvailable != (tc.state != "unknown") || body.Data.BillingOrderID != "" {
			t.Fatalf("task funds = %s", w.Body.String())
		}
		f.request(t, "GET", "/api/tasks/"+tc.id, "platform_session=bob", "", 404)
	}
	w := f.request(t, "GET", "/api/tasks", "platform_session=alice", "", 200)
	var list struct {
		Data []struct {
			ID              string
			PlatformBilling *model.StudioTaskFunds
			Billing         json.RawMessage
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Data) != 4 {
		t.Fatalf("task list = %s (%v)", w.Body.String(), err)
	}
	for _, task := range list.Data {
		if task.PlatformBilling == nil || len(task.Billing) != 0 {
			t.Fatalf("task list contains local billing: %#v", task)
		}
	}
	for _, table := range []string{"billing_orders", "credit_accounts", "credit_ledger_entries"} {
		if f.db.Migrator().HasTable(table) {
			t.Fatalf("task funds created retired table %s", table)
		}
	}
}
