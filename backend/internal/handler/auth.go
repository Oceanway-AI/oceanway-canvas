package handler

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/service"
)

func currentUser(c *gin.Context, svc *service.Service) (*model.User, error) {
	if svc.UsesPlatformIdentity() {
		if err := svc.ValidateStudioRequest(c.Request); err != nil {
			return nil, err
		}
		started := time.Now()
		user, response, err := svc.StudioCurrentUser(c.Request.Context(), platformintegration.Session(c.Request))
		if c.Request.Method == http.MethodPost && c.Request.URL.Path == "/api/tasks" {
			var providerTrace string
			if response != nil {
				providerTrace = normalizeCorrelationID(response.Header.Get("X-Oneapi-Request-Id"))
			}
			slog.InfoContext(c.Request.Context(), "studio task identity", "request_id", RequestID(c), "provider_trace", providerTrace, "start_us", started.UnixMicro(), "end_us", time.Now().UnixMicro())
		}
		if response != nil && !c.Writer.Written() {
			response.AppendCookies(c.Writer.Header())
		}
		c.Header("Cache-Control", "no-store")
		if err == nil && c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && !strings.HasSuffix(c.FullPath(), "/auth/session") {
			if err := svc.ValidateStudioActor(user, c.GetHeader("X-Studio-Actor-ID")); err != nil {
				return nil, err
			}
		}
		return user, err
	}
	return nil, service.Unauthorized("请登录 OceanWay 账号")
}
