package handler

import (
	"errors"
	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"github.com/gin-gonic/gin"
	"strconv"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/service"
)

func studioCatalogReadHeaders(c *gin.Context, response *platformclient.Response, err error) {
	response.AppendCookies(c.Writer.Header())
	var appErr *service.AppError
	if response == nil || !errors.As(err, &appErr) || appErr.Reason != "catalog_projection_pending" {
		return
	}
	values := response.Header.Values("Retry-After")
	if len(values) == 1 {
		if n, e := strconv.Atoi(values[0]); e == nil && n >= 1 && n <= 60 && strconv.Itoa(n) == values[0] {
			c.Header("Retry-After", values[0])
		}
	}
}

func registerStudioCatalogRoutes(api *gin.RouterGroup, svc *service.Service) {
	api.GET("/model-catalog/summaries", func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		if _, err := currentUser(c, svc); err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		var query struct {
			WorkspaceID  string `form:"workspace_id"`
			ModelType    string `form:"model_type"`
			DefaultsOnly bool   `form:"defaults_only"`
			Query        string `form:"query"`
			Cursor       string `form:"cursor"`
			Limit        int    `form:"limit" binding:"min=0,max=50"`
		}
		if c.ShouldBindQuery(&query) != nil {
			failService(c, service.BadAuthRequest("模型目录查询参数无效"))
			return
		}
		catalog, response, err := svc.StudioModelCatalogPage(c.Request.Context(), platformintegration.Session(c.Request), platformclient.ModelSummaryQuery{WorkspaceID: query.WorkspaceID, ModelType: query.ModelType, DefaultsOnly: query.DefaultsOnly, Query: query.Query, Cursor: query.Cursor, Limit: query.Limit})
		studioCatalogReadHeaders(c, response, err)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, catalog)
	})
	api.GET("/model-catalog/detail", func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		var query struct {
			WorkspaceID string `form:"workspace_id"`
			ModelName   string `form:"model_name" binding:"required"`
			VariantID   string `form:"variant_id"`
			Price       string `form:"expected_price_version" binding:"required"`
			Capability  string `form:"expected_capability_revision"`
			Connection  int64  `form:"expected_connection_revision" binding:"min=0"`
		}
		if c.ShouldBindQuery(&query) != nil {
			failService(c, service.BadAuthRequest("模型详情查询参数无效"))
			return
		}
		item, response, err := svc.StudioModelCatalogDetail(c.Request.Context(), platformintegration.Session(c.Request), platformclient.ModelDetailQuery{WorkspaceID: query.WorkspaceID, ModelName: query.ModelName, VariantID: query.VariantID, ExpectedPriceVersion: query.Price, ExpectedCapabilityRevision: query.Capability, ExpectedConnectionRevision: query.Connection}, user)
		studioCatalogReadHeaders(c, response, err)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"model": item})
	})
}
