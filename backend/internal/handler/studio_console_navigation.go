package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/service"
)

func registerStudioConsoleNavigation(api *gin.RouterGroup, svc *service.Service) {
	api.GET("/auth/console-navigation", func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("Referrer-Policy", "no-referrer")
		query := c.Request.URL.Query()
		for _, name := range []string{"user_id", "workspace_id", "destination", "return_path"} {
			if len(query[name]) != 1 || query.Get(name) == "" || len(query) != 4 {
				failService(c, service.NewAppError(400, "账户导航参数无效"))
				return
			}
		}
		target, response, err := svc.StudioConsoleNavigation(c.Request.Context(), platformintegration.Session(c.Request), query.Get("user_id"), query.Get("workspace_id"), query.Get("destination"), query.Get("return_path"))
		response.AppendCookies(c.Writer.Header())
		if err != nil {
			failService(c, err)
			return
		}
		c.Redirect(http.StatusFound, target)
	})
}
