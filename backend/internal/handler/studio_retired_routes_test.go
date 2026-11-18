package handler

import "testing"

func TestStudioRetiredServicesHaveNoRoutes(t *testing.T) {
	f := newStudioFixture(t)
	f.admin = true
	for _, endpoint := range []struct{ method, path string }{
		{"POST", "/api/auth/register"},
		{"POST", "/api/auth/login"},
		{"POST", "/api/auth/email-code"},
		{"POST", "/api/auth/password-reset"},
		{"GET", "/api/admin/users"},
		{"GET", "/api/admin/login-logs"},
		{"GET", "/api/admin/agent-lessons"},
		{"DELETE", "/api/admin/agent-lessons/private-memory"},
		{"GET", "/api/admin/agent-memory-documents"},
		{"GET", "/api/wallet"},
		{"POST", "/api/credits/checkin"},
		{"POST", "/api/credits/redeem"},
		{"POST", "/api/payment/orders"},
		{"GET", "/api/referral"},
		{"POST", "/api/admin/channels/id/models/batch-reprice"},
		{"POST", "/api/admin/settings/oss/test"},
		{"POST", "/api/admin/api-logs/legacy/recover-media"},
		{"POST", "/api/settings/oss/test"},
		{"POST", "/api/resources/uploads"},
		{"POST", "/api/feedbacks"},
		{"GET", "/api/admin/feedbacks"},
		{"POST", "/api/ecommerce/projects"},
		{"GET", "/api/ecommerce/download?resourceId=foreign"},
		{"GET", "/api/admin/ecommerce/categories"},
		{"GET", "/api/admin/ecommerce/skills"},
		{"POST", "/api/agent/generate/stream"},
		{"POST", "/api/projects"},
	} {
		for _, cookie := range []string{"", "platform_session=alice"} {
			f.request(t, endpoint.method, endpoint.path, cookie, "{}", 501)
		}
	}
}
