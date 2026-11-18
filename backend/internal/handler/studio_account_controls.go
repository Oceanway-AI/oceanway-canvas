package handler

import (
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/service"
)

func registerStudioAccountControls(api *gin.RouterGroup, svc *service.Service) {
	api.GET("/auth/workspaces", func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		result, response, err := svc.StudioWorkspaceChoices(c.Request.Context(), platformintegration.Session(c.Request), user)
		response.AppendCookies(c.Writer.Header())
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
	api.GET("/auth/product-navigation", func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		query, err := url.ParseQuery(c.Request.URL.RawQuery)
		valid := err == nil && len(query) == 4
		for _, key := range []string{"user_id", "workspace_id", "membership_generation", "product"} {
			valid = valid && len(query[key]) == 1 && query.Get(key) != ""
		}
		generation, parseErr := strconv.ParseInt(query.Get("membership_generation"), 10, 64)
		if !valid || parseErr != nil || generation < 0 || generation > 9007199254740991 {
			failService(c, service.BadAuthRequest("产品导航参数无效"))
			return
		}
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		target, response, err := svc.StudioProductNavigation(c.Request.Context(), platformintegration.Session(c.Request), user, query.Get("user_id"), query.Get("workspace_id"), query.Get("product"), generation)
		response.AppendCookies(c.Writer.Header())
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"url": target, "userId": user.ID, "workspaceId": query.Get("workspace_id"), "membershipGeneration": generation})
	})
}
