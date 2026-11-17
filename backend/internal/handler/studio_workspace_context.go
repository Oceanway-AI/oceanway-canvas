package handler

import (
	"net/url"
	"slices"
	"strconv"

	"github.com/gin-gonic/gin"
	"yingce/backend/internal/service"
)

func studioWorkspaceIntent(c *gin.Context) (string, string, *int64, error) {
	q, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		return "", "", nil, service.NewAppError(400, "空间参数无效")
	}
	for key := range q {
		if !slices.Contains([]string{"workspace_id", "canvas_id", "membership_generation"}, key) {
			return "", "", nil, service.NewAppError(400, "空间参数无效")
		}
	}
	for _, key := range []string{"workspace_id", "canvas_id", "membership_generation"} {
		if values, ok := q[key]; ok && (len(values) != 1 || values[0] == "") {
			return "", "", nil, service.NewAppError(400, "空间参数无效")
		}
	}
	var generation *int64
	if q.Has("membership_generation") {
		value, err := strconv.ParseInt(q.Get("membership_generation"), 10, 64)
		if err != nil || value < 0 || q.Get("workspace_id") == "" {
			return "", "", nil, service.NewAppError(400, "空间参数无效")
		}
		generation = &value
	}
	return q.Get("workspace_id"), q.Get("canvas_id"), generation, nil
}
