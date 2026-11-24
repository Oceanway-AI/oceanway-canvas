package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/service"
)

func registerStudioTeamCanvases(api *gin.RouterGroup, svc *service.Service) {
	canvas := api.Group("/workspaces/:wid/canvas-projects")
	handle := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		values := c.Request.URL.Query()["membership_generation"]
		if len(values) != 1 {
			failService(c, service.BadAuthRequest("需要团队成员版本"))
			return
		}
		generation, err := strconv.ParseInt(values[0], 10, 64)
		if err != nil || generation <= 0 {
			failService(c, service.BadAuthRequest("团队成员版本无效"))
			return
		}
		entry, resp, err := svc.StudioWorkspaceContext(c.Request.Context(), platformintegration.Session(c.Request), user, c.Param("wid"), "", &generation)
		resp.AppendCookies(c.Writer.Header())
		if err != nil {
			failService(c, err)
			return
		}
		key, err := entry.TeamCanvasKey()
		if err != nil {
			failService(c, err)
			return
		}
		id := c.Param("id")
		var out any
		switch {
		case c.Request.Method == http.MethodPatch:
			policy, enabled := loadRuntimePolicy(c, svc)
			if !enabled || !enforceRateLimit(c, "canvas-write:"+user.ID, policy.Request.CanvasWritePerMinute, time.Minute) {
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 5<<20)
			var req service.CanvasOperationsRequest
			if c.ShouldBindJSON(&req) != nil {
				failService(c, service.BadAuthRequest("画布操作格式错误"))
				return
			}
			project, e := svc.StudioTeamCanvasOperations(user, entry, id, req)
			out, err = gin.H{"project": project, "mutationId": req.MutationID}, e
		case c.Request.Method == http.MethodPost:
			if !enforceRateLimit(c, "canvas-lease:"+user.ID, 240, time.Minute) {
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
			var req service.CanvasEditLeaseRequest
			if c.ShouldBindJSON(&req) != nil {
				failService(c, service.BadAuthRequest("编辑租约格式错误"))
				return
			}
			out, err = svc.StudioTeamCanvasLease(user, entry, id, req)
		case id == "":
			rows, e := svc.UserCanvasProjectSummaries(key)
			out, err = gin.H{"projects": rows}, e
		case c.FullPath() == canvas.BasePath()+"/:id/sync-status":
			out, err = svc.UserCanvasSyncStatus(key, id)
		default:
			project, e := svc.UserCanvasProject(key, id)
			out, err = gin.H{"project": project}, e
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = service.NotFound("画布不存在")
		}
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, out)
	}
	canvas.GET("", handle)
	canvas.GET("/:id", handle)
	canvas.GET("/:id/sync-status", handle)
	canvas.PATCH("/:id/operations", handle)
	canvas.POST("/:id/edit-lease", handle)
}
