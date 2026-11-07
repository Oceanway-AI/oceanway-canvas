package handler

import (
	"net/http"
	"strconv"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"github.com/gin-gonic/gin"
	"yingce/backend/internal/assets"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/service"
)

func studioMaterialReply(c *gin.Context, data any, response *platformclient.Response, err error) {
	if response != nil {
		response.AppendCookies(c.Writer.Header())
	}
	if err != nil {
		failService(c, err)
		return
	}
	ok(c, data)
}

func studioMaterialJSON(c *gin.Context, in any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	if err := c.ShouldBindJSON(in); err != nil {
		failService(c, service.BadAuthRequest("素材请求格式无效"))
		return false
	}
	return true
}

func RegisterStudioMaterialRoutes(api *gin.RouterGroup, svc *service.Service) {
	api.POST("/resources/platform-uploads", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var in service.StudioUploadRequest
		if !studioMaterialJSON(c, &in) {
			return
		}
		data, response, err := svc.StudioCreateUpload(c.Request.Context(), user.ID, platformintegration.Session(c.Request), in)
		studioMaterialReply(c, data, response, err)
	})
	api.GET("/resources/platform-uploads/:id", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		data, response, err := svc.StudioUploadStatus(c.Request.Context(), user.ID, platformintegration.Session(c.Request), c.Param("id"), nil)
		studioMaterialReply(c, data, response, err)
	})
	api.POST("/resources/platform-uploads/:id/complete", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var in platformclient.UploadComplete
		if !studioMaterialJSON(c, &in) {
			return
		}
		data, response, err := svc.StudioUploadStatus(c.Request.Context(), user.ID, platformintegration.Session(c.Request), c.Param("id"), &in)
		studioMaterialReply(c, data, response, err)
	})
	api.POST("/resources/adopt", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var in service.StudioAdoptRequest
		if !studioMaterialJSON(c, &in) {
			return
		}
		data, response, err := svc.StudioAdoptMaterial(c.Request.Context(), user.ID, platformintegration.Session(c.Request), in)
		studioMaterialReply(c, gin.H{"resource": data}, response, err)
	})
	api.POST("/resources/access", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var in []service.ResourceAccessRequest
		if !studioMaterialJSON(c, &in) {
			return
		}
		data, response, err := svc.StudioResourceAccess(c.Request.Context(), user, platformintegration.Session(c.Request), in)
		studioMaterialReply(c, gin.H{"items": data}, response, err)
	})
	api.GET("/resources/:id", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		data, err := svc.StudioResource(c.Request.Context(), user.ID, c.Param("id"))
		studioMaterialReply(c, gin.H{"resource": data}, nil, err)
	})
	api.GET("/resources/:id/file", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		items, response, err := svc.StudioResourceAccess(c.Request.Context(), user, platformintegration.Session(c.Request), []service.ResourceAccessRequest{{ResourceID: c.Param("id"), AccessOptions: assets.AccessOptions{Purpose: assets.AccessPurpose(c.DefaultQuery("purpose", "display")), Variant: assets.ResourceVariant(c.DefaultQuery("variant", "original"))}}})
		if response != nil {
			response.AppendCookies(c.Writer.Header())
		}
		if err != nil {
			failService(c, err)
			return
		}
		if len(items) != 1 || items[0].Access == nil {
			if len(items) == 1 && items[0].Error != nil {
				failService(c, service.NewAppError(items[0].Error.Code, items[0].Error.Message))
				return
			}
			failService(c, service.NewAppError(503, "素材暂时不可访问"))
			return
		}
		c.Header("Referrer-Policy", "no-referrer")
		c.Redirect(http.StatusTemporaryRedirect, items[0].Access.URL)
	})
	api.GET("/materials/assets", func(c *gin.Context) {
		if _, err := currentUser(c, svc); err != nil {
			failService(c, err)
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		if page < 1 || limit < 1 || limit > 100 {
			failService(c, service.BadAuthRequest("素材分页参数无效"))
			return
		}
		data, response, err := svc.StudioMaterialAssets(c.Request.Context(), platformintegration.Session(c.Request), platformclient.AssetQuery{Page: page, Limit: limit, Query: c.Query("q"), Kind: c.Query("kind")})
		studioMaterialReply(c, data, response, err)
	})
	api.GET("/materials/storage", func(c *gin.Context) {
		if _, err := currentUser(c, svc); err != nil {
			failService(c, err)
			return
		}
		data, response, err := svc.StudioMaterialStorage(c.Request.Context(), platformintegration.Session(c.Request))
		studioMaterialReply(c, data, response, err)
	})
}
