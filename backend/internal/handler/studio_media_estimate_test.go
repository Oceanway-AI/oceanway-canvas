package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"yingce/backend/internal/model"
)

func TestStudioStandardMediaEstimateKeepsUnknownZeroAndPermissionSeparate(t *testing.T) {
	f := newStudioFixture(t)
	price := strings.Repeat("e", 64)
	variant := strings.Repeat("b", 64)
	f.catalog = strings.NewReplacer("price-1", price, `"id":"generate"`, `"id":"`+variant+`"`, `"default_variant":"generate"`, `"default_variant":"`+variant+`"`).Replace(studioGenerationCatalog)
	request := strings.ReplaceAll(studioGenerationInput, `"commandId":"original-v1",`, "")
	request = strings.Replace(request, `"expectedPriceVersion":"price-1","walletMaxPoints":"0"`, `"mediaGenerationV1":true,"variantId":"`+variant+`","expectedPriceVersion":"`+price+`"`, 1)
	calls := 0
	state := "available"
	var points *string
	zero := "0"
	points = &zero
	f.execution = func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/platform/v1/products/studio/media/price-preview" || r.Header.Get("Cookie") == "" || r.Header.Get("X-Platform-Product-Key") == "" {
			t.Errorf("unexpected estimate or identity: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		calls++
		var in map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatal(err)
		}
		if string(in["variant_id"]) != `"`+variant+`"` || string(in["expected_price_version"]) != `"`+price+`"` || !strings.Contains(string(in["native_request"]), `"prompt":"cat"`) || in["wallet_max_points"] != nil || in["command_id"] != nil {
			t.Error("standard estimate must carry the selected price, variant and exact native input alone")
		}
		w.Header().Set("Content-Type", "application/json")
		data := map[string]any{"state": state, "model": "exact-image", "model_type": "image", "variant_id": variant, "capability_revision": strings.Repeat("c", 64), "connection_revision": 4, "price_version": price, "unit": "points", "calculated_at": 1794810000}
		if points != nil {
			data["points"] = *points
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
	}
	const session = "platform_session=alice"
	f.request(t, "GET", "/api/model-catalog", session, "", http.StatusOK)
	for _, expected := range []string{"available", "unavailable"} {
		state = expected
		if state == "unavailable" {
			points = nil
		}
		res := f.request(t, "POST", "/api/tasks/estimate", session, request, http.StatusOK)
		var body struct{ Data map[string]any }
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Data["state"] != state || body.Data["priceVersion"] != price || body.Data["variantId"] != variant || body.Data["wallet_points"] != nil || body.Data["allowed"] != nil || body.Data["previewBinding"] != nil {
			t.Fatalf("unexpected estimate %v", body.Data)
		}
		if (state == "available" && body.Data["points"] != "0") || (state == "unavailable" && body.Data["points"] != nil) {
			t.Fatalf("unknown/zero price lost: %v", body.Data)
		}
	}
	f.request(t, "POST", "/api/tasks/estimate", session, strings.Replace(request, `"mediaGenerationV1":true`, `"workspaceId":"team_other","mediaGenerationV1":true`, 1), http.StatusForbidden)
	f.request(t, "POST", "/api/tasks/estimate", session, strings.Replace(request, price, strings.Repeat("f", 64), 1), http.StatusConflict)
	if calls != 2 {
		t.Fatalf("estimate calls=%d", calls)
	}
	if f.db.Migrator().HasTable(&model.BillingOrder{}) {
		t.Fatal("Studio created local billing")
	}
	for _, table := range []any{&model.Task{}, &model.StudioExecution{}} {
		var rows int64
		if err := f.db.Model(table).Count(&rows).Error; err != nil || rows != 0 {
			t.Fatalf("standard estimate wrote %T: %d %v", table, rows, err)
		}
	}
}
