package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/service"
)

func RegisterStudioTaskRoutes(r *gin.RouterGroup, svc *service.Service) {
	r.POST("/tasks/estimate", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20)
		var req service.CreateTaskRequest
		if c.ShouldBindJSON(&req) != nil {
			failService(c, service.BadAuthRequest("估价参数无效"))
			return
		}
		price, response, err := svc.StudioEstimateMediaPrice(c.Request.Context(), user.ID, platformintegration.Session(c.Request), req, user.PlatformSessionID)
		response.AppendCookies(c.Writer.Header())
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, price)
	})
	r.POST("/tasks/preview", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20)
		var req service.CreateTaskRequest
		if c.ShouldBindJSON(&req) != nil {
			failService(c, service.BadAuthRequest("任务输入无效"))
			return
		}
		quote, response, err := svc.StudioPreviewTask(c.Request.Context(), user.ID, platformintegration.Session(c.Request), req, user.PlatformSessionID)
		response.AppendCookies(c.Writer.Header())
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, quote)
	})
	r.POST("/tasks", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		policy, available := loadRuntimePolicy(c, svc)
		if !available || !enforceRateLimit(c, "tasks:"+user.ID, policy.Request.TaskCreatePerMinute, time.Minute) {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20)
		var req service.CreateTaskRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			failService(c, service.BadAuthRequest("任务输入无效"))
			return
		}
		req.TraceID, req.RequestID = TraceID(c), RequestID(c)
		task, response, err := svc.StudioCreateTask(c.Request.Context(), user.ID, platformintegration.Session(c.Request), req, user.PlatformSessionID)
		response.AppendCookies(c.Writer.Header())
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, task)
	})
	r.GET("/tasks/by-command/:command", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		task, err := svc.StudioTaskByCommand(user.ID, c.Param("command"), c.Query("expectedUserId"))
		if err != nil {
			failService(c, err)
			return
		}
		task, response, err := svc.StudioReadGeneration(c.Request.Context(), task, platformintegration.Session(c.Request))
		response.AppendCookies(c.Writer.Header())
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, task)
	})
	r.GET("/tasks", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		limit, err := parsePositiveQueryInt(c.Query("pageSize"), 50)
		if err != nil {
			fail(c, 400, err)
			return
		}
		tasks, err := svc.TasksWithOptions(user.ID, service.TaskListOptions{Limit: min(limit, 100), ProjectID: c.Query("projectId"), ActiveOnly: c.Query("activeOnly") == "true"})
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, tasks)
	})
	r.GET("/tasks/:id", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		task, err := svc.Task(user.ID, c.Param("id"))
		if err != nil {
			fail(c, 404, err)
			return
		}
		task, response, err := svc.StudioReadGeneration(c.Request.Context(), task, platformintegration.Session(c.Request))
		response.AppendCookies(c.Writer.Header())
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, task)
	})
	r.POST("/tasks/:id/adopt-outputs", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		task, err := svc.Task(user.ID, c.Param("id"))
		if err != nil {
			fail(c, 404, err)
			return
		}
		task, response, err := svc.StudioAdoptGenerationOutputs(c.Request.Context(), task, platformintegration.Session(c.Request))
		response.AppendCookies(c.Writer.Header())
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, task)
	})
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		r.Handle(method, "/tasks/:id/previews/:index", func(c *gin.Context) {
			c.Header("Cache-Control", "private, no-store")
			c.Header("Referrer-Policy", "no-referrer")
			user, err := currentUser(c, svc)
			if err != nil {
				failService(c, err)
				return
			}
			raw := c.Param("index")
			index, err := strconv.Atoi(raw)
			if err != nil || index < 0 || index >= 128 || strconv.Itoa(index) != raw {
				failService(c, service.NewAppError(404, "任务预览不存在"))
				return
			}
			c.Header("Cache-Control", "private, no-store")
			target, response, err := svc.StudioGenerationPreview(c.Request.Context(), user.ID, c.Param("id"), index, platformintegration.Session(c.Request))
			response.AppendCookies(c.Writer.Header())
			if err != nil {
				failService(c, err)
				return
			}
			c.Redirect(http.StatusTemporaryRedirect, target)
		})
	}
	r.GET("/tasks/:id/logs", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		logs, err := svc.TaskLogs(user.ID, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, logs)
	})
	r.GET("/tasks/:id/text-deltas", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		after, err := taskTextEventCursor(c)
		if err != nil {
			fail(c, 400, err)
			return
		}
		replay, err := svc.TaskTextReplay(user.ID, c.Param("id"), after)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, replay)
	})
	r.GET("/tasks/:id/text-events", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		after, err := taskTextEventCursor(c)
		if err != nil {
			fail(c, 400, err)
			return
		}
		replay, err := svc.CachedTaskTextReplay(c.Request.Context(), user.ID, c.Param("id"), after)
		if err != nil {
			failService(c, err)
			return
		}
		streamTaskTextEvents(c, svc, user.ID, c.Param("id"), after, replay)
	})
}
