package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/service"
)

// RegisterStudioAPI is the production surface during the staged replacement.
// A domain is added here only when its Platform boundary is implemented. The
// upstream RegisterCanvasAPI remains available to existing domain test fixtures.
func RegisterStudioAPI(api *gin.RouterGroup, svc *service.Service) error {
	if !svc.UsesPlatformIdentity() {
		return errors.New("Studio API requires Platform identity")
	}
	api.Use(func(c *gin.Context) {
		if err := svc.ValidateStudioRequest(c.Request); err != nil {
			failService(c, err)
			c.Abort()
			return
		}
		c.Next()
	})
	api.GET("/auth/settings", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		ok(c, gin.H{"provider": "platform", "loginMode": "page", "registrationEnabled": false, "firstUser": false})
	})
	api.GET("/auth/login-url", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Referrer-Policy", "no-referrer")
		target, err := svc.StudioLoginURL(c.Request.Context())
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"url": target})
	})
	api.GET("/auth/profile", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"user": user, "provider": "platform", "consoleURL": svc.StudioConsoleURL(), "hasPassword": false, "emailEnabled": false, "linuxdoEnabled": false})
	})
	api.POST("/auth/logout", func(c *gin.Context) {
		response, err := svc.StudioLogout(c.Request.Context(), platformintegration.Session(c.Request))
		if response != nil {
			response.AppendCookies(c.Writer.Header())
		}
		c.Header("Cache-Control", "no-store")
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"ok": true})
	})
	api.GET("/auth/session", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			var appErr *service.AppError
			if errors.As(err, &appErr) && appErr.Status == http.StatusUnauthorized {
				ok(c, gin.H{"user": nil})
				return
			}
			failService(c, err)
			return
		}
		limits, err := svc.PublicRuntimeLimits()
		if err != nil {
			failService(c, err)
			return
		}
		drawing, err := svc.DrawingEngineSetting()
		if err != nil {
			failService(c, err)
			return
		}
		features, err := studioFeatures(svc)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"user": user, "runtimeLimits": limits, "drawingEngine": drawing, "features": features, "referralAvailable": false})
	})
	api.GET("/public/appearance", func(c *gin.Context) {
		appearance, err := svc.Appearance()
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"appearance": appearance})
	})
	api.GET("/public/welcome", func(c *gin.Context) { ok(c, gin.H{"welcomeEnabled": false}) })
	api.GET("/features", func(c *gin.Context) {
		if _, err := currentUser(c, svc); err != nil {
			failService(c, err)
			return
		}
		features, err := studioFeatures(svc)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"features": features})
	})
	RegisterCanvasDataRoutes(api, svc)
	RegisterCanvasRealtimeRoutes(api, svc)
	RegisterCanvasShareRoutes(api, svc)
	return nil
}

func studioFeatures(svc *service.Service) (*service.PublicFeatureAvailability, error) {
	features, err := svc.FeatureAvailability()
	if err != nil {
		return nil, err
	}
	features.CreditsEnabled = false
	features.CustomChannelsEnabled = false
	features.FrontendModelsEnabled = false
	features.WelcomeEnabled = false
	return features, nil
}

func StudioUnavailableRoute(c *gin.Context) {
	failService(c, service.NewAppError(http.StatusNotImplemented, "此功能暂不可用"))
}
