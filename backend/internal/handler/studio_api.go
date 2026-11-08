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
	sessionHandler := func(c *gin.Context) {
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
		if c.Request.Method == http.MethodPost {
			response, err := svc.StudioAuthorizeMaterials(c.Request.Context(), user, platformintegration.Session(c.Request))
			if response != nil {
				response.AppendCookies(c.Writer.Header())
			}
			if err != nil {
				failService(c, err)
				return
			}
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
		ok(c, gin.H{"user": user, "provider": "platform", "runtimeLimits": limits, "drawingEngine": drawing, "features": features, "referralAvailable": false})
	}
	api.GET("/auth/session", sessionHandler)
	api.POST("/auth/session", sessionHandler)
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
	api.GET("/model-catalog", func(c *gin.Context) {
		if _, err := currentUser(c, svc); err != nil {
			failService(c, err)
			return
		}
		catalog, response, err := svc.StudioModelCatalog(c.Request.Context(), platformintegration.Session(c.Request))
		if response != nil {
			response.AppendCookies(c.Writer.Header())
		}
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, catalog)
	})
	RegisterStudioMaterialRoutes(api, svc)
	RegisterAssetLibraryRoutes(api, svc)
	RegisterStudioTaskRoutes(api, svc)
	RegisterAgentRoutes(api, svc)
	RegisterAgentSkillRoutes(api, svc)
	RegisterCanvasDataRoutes(api, svc)
	RegisterCanvasRealtimeRoutes(api, svc)
	// Public material delivery needs a distinct Platform sharing authorization.
	// Do not expose the upstream share/token or local resource delivery paths.
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
	features.ShortDramaEnabled = false
	features.EcommerceEnabled = false
	features.EcommerceVideoEnabled = false
	features.ImageWorkbenchEnabled = false
	features.VideoWorkbenchEnabled = false
	features.PluginCenterEnabled = false
	features.SystemPluginsVisibleToUsers = false
	features.InspirationSourcesVisible = false
	features.TimelineTranscriptionEnabled = false
	return features, nil
}

func StudioUnavailableRoute(c *gin.Context) {
	failService(c, service.NewAppError(http.StatusNotImplemented, "此功能暂不可用"))
}
