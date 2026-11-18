package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterOpenAPIRoutes(api *gin.RouterGroup) {
	api.GET("/openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml; charset=utf-8", openAPISpec)
	})
}
