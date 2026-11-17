package handler

import (
	"strings"
	"testing"
)

func TestStudioSummaryAndDetailUseAuthenticatedHTTP(t *testing.T) {
	f := newStudioFixture(t)
	f.catalog = `{"success":true,"data":{"items":[{"model_name":"text-model","model_type":"text","invocation_status":"published_text","supported_protocols":["openai-chat"],"price":{"status":"available","version":"p1","rates":{"input":"0.00000000123456789"}},"media_generation_profile":{"extension":9007199254740993}}]}}`
	page := f.request(t, "GET", "/api/model-catalog/summaries?limit=20", "platform_session=alice", "", 200)
	if strings.Contains(page.Body.String(), "media_generation_profile") || !strings.Contains(page.Body.String(), `"state":"summary"`) || page.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("summary leaked profile or lost cache isolation: cache=%q body=%s", page.Header().Get("Cache-Control"), page.Body.String())
	}
	if !strings.Contains(page.Header().Get("Set-Cookie"), "catalog_nonce=") {
		t.Fatal("catalog cookie rotation was lost")
	}
	detail := f.request(t, "GET", "/api/model-catalog/detail?model_name=text-model&expected_price_version=p1", "platform_session=alice", "", 200)
	if !strings.Contains(detail.Body.String(), "9007199254740993") || !strings.Contains(detail.Body.String(), "0.00000000123456789") || !strings.Contains(detail.Body.String(), `"state":"detail"`) {
		t.Fatal("detail lost exact extensions, prices or detail state")
	}
	f.request(t, "GET", "/api/model-catalog/detail?model_name=text-model&expected_price_version=old", "platform_session=alice", "", 409)
	reads := f.catalogCalls
	f.request(t, "GET", "/api/model-catalog/summaries", "", "", 401)
	f.request(t, "GET", "/api/model-catalog/detail?model_name=text-model&expected_price_version=p1", "", "", 401)
	if f.catalogCalls != reads {
		t.Fatal("anonymous read reached platform")
	}
	f.request(t, "GET", "/api/model-catalog/summaries?limit=51", "platform_session=alice", "", 400)
	f.request(t, "GET", "/api/model-catalog/detail?model_name=text-model", "platform_session=alice", "", 400)
}
