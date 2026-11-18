package handler

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
	"yingce/backend/internal/model"
)

func TestTaskMediaRecoveryHTTPAuthenticationAndOwnership(t *testing.T) {
	f := newStudioFixture(t)
	db := f.db
	for _, item := range []any{
		&model.Task{ID: "legacy-task", UserID: "101", Status: model.TaskStatusFailed},
		&model.Task{ID: "foreign-task", UserID: "other-user", Status: model.TaskStatusFailed},
	} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := gin.New()
	RegisterTaskRoutes(router.Group("/api"), f.svc)
	for _, tc := range []struct {
		id   string
		auth bool
		want int
	}{
		{"legacy-task", false, http.StatusUnauthorized},
		{"legacy-task", true, http.StatusBadRequest},
		{"foreign-task", true, http.StatusNotFound},
		{"missing-task", true, http.StatusNotFound},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/tasks/"+tc.id+"/recover-media", nil)
		if tc.auth {
			r.AddCookie(&http.Cookie{Name: "platform_session", Value: "alice"})
		}
		r.Host = "studio.example.test"
		r.Header.Set("Origin", "https://studio.example.test")
		r.Header.Set("X-Studio-Identity-Version", "platform-id-v1")
		r.Header.Set("X-Studio-Actor-ID", "101")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s auth=%v: %d %s", tc.id, tc.auth, w.Code, w.Body.String())
		}
	}
}
