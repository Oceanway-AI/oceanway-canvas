// Package platformintegration defines Studio's trusted Platform configuration.
// Business orchestration stays in app; provider contracts stay in platformclient.
package platformintegration

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

// NewFromEnv never selects local authentication or a supplier fallback.
// The injected getter permits tests without reading developer credentials.
func NewFromEnv(getenv func(string) string) (*platformclient.Client, error) {
	origin := strings.TrimSpace(getenv("STUDIO_PLATFORM_ORIGIN"))
	key := getenv("STUDIO_PLATFORM_PRODUCT_KEY")
	hosts := splitList(getenv("STUDIO_PLATFORM_SESSION_HOSTS"))
	origins := splitList(getenv("STUDIO_PLATFORM_ALLOWED_ORIGINS"))
	if origin == "" || strings.TrimSpace(key) == "" || len(hosts) == 0 || len(origins) == 0 {
		return nil, errors.New("Studio Platform configuration requires origin, product key, session hosts and allowed origins")
	}
	return platformclient.NewClient(platformclient.Config{
		Origin: origin, Product: "studio", ProductKey: key,
		AllowedSessionHosts: hosts, AllowedOrigins: origins,
	})
}

// Session uses the actual inbound authority and preserves a missing Origin.
// Reverse proxies must preserve Host; forwarded headers cannot choose identity.
func Session(r *http.Request) platformclient.SessionContext {
	return platformclient.SessionContext{
		Host: r.Host, Origin: r.Header.Get("Origin"), Cookie: r.Header.Get("Cookie"),
	}
}

func splitList(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}
