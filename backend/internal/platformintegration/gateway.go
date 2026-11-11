package platformintegration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

// Gateway keeps the published SDK and anonymous login-return contract together.
// Login preparation never carries a product key or browser cookies.
type Gateway struct {
	Client        *platformclient.Client
	origin        string
	browserOrigin string
	publicOrigin  string
	hosts         []string
	origins       []string
	http          *http.Client
}

func NewGatewayFromEnv(getenv func(string) string) (*Gateway, error) {
	client, err := NewFromEnv(getenv)
	if err != nil {
		return nil, err
	}
	publicOrigin := strings.TrimSpace(getenv("STUDIO_PUBLIC_ORIGIN"))
	u, err := url.Parse(publicOrigin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("STUDIO_PUBLIC_ORIGIN must be an exact HTTPS origin")
	}
	hosts := splitList(getenv("STUDIO_PLATFORM_SESSION_HOSTS"))
	origins := splitList(getenv("STUDIO_PLATFORM_ALLOWED_ORIGINS"))
	if !contains(hosts, u.Host) || !contains(origins, publicOrigin) {
		return nil, errors.New("Studio public origin must match its registered session host and origin")
	}
	origin := strings.TrimSpace(getenv("STUDIO_PLATFORM_ORIGIN"))
	browserOrigin := strings.TrimSpace(getenv("STUDIO_PLATFORM_BROWSER_ORIGIN"))
	if browserOrigin == "" {
		browserOrigin = origin
	} else {
		target, err := url.Parse(browserOrigin)
		if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil || target.Path != "" || strings.ContainsAny(browserOrigin, "?#") {
			return nil, errors.New("STUDIO_PLATFORM_BROWSER_ORIGIN must be an exact HTTPS origin")
		}
	}
	return &Gateway{Client: client, origin: origin, browserOrigin: browserOrigin, publicOrigin: publicOrigin,
		hosts: hosts, origins: origins, http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (g *Gateway) Origin() string        { return g.origin }
func (g *Gateway) BrowserOrigin() string { return g.browserOrigin }

func (g *Gateway) ValidateInbound(r *http.Request) error {
	if !contains(g.hosts, r.Host) {
		return errors.New("unregistered Studio host")
	}
	origin := r.Header.Get("Origin")
	if origin != "" && !contains(g.origins, origin) {
		return errors.New("unregistered Studio origin")
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && origin == "" {
		return errors.New("Studio mutations require Origin")
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// LoginURL uses the fixed callback accepted by the provider. Product navigation
// stays in the browser; arbitrary next URLs never enter the provider contract.
func (g *Gateway) LoginURL(ctx context.Context) (string, error) {
	callback := g.publicOrigin + "/platform/login/complete"
	endpoint := g.origin + "/api/platform/v1/auth/login-return?" + url.Values{"return_url": {callback}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", errors.New("invalid Platform login configuration")
	}
	response, err := g.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return "", context.DeadlineExceeded
		}
		return "", errors.New("Platform login preparation unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", errors.New("Platform did not approve the Studio login return")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil || len(body) > 64*1024 {
		return "", errors.New("invalid Platform login response")
	}
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			ReturnURL string `json:"return_url"`
			LoginPath string `json:"login_path"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &result) != nil || !result.Success || result.Data.ReturnURL != callback {
		return "", errors.New("invalid Platform login return")
	}
	path, err := url.Parse(result.Data.LoginPath)
	if err != nil || path.IsAbs() || path.Host != "" || path.Path != "/login" || path.RawPath != "" || path.Fragment != "" {
		return "", errors.New("invalid Platform login path")
	}
	query, err := url.ParseQuery(path.RawQuery)
	if err != nil || len(query) != 1 || len(query["return"]) != 1 || query.Get("return") != callback {
		return "", errors.New("invalid Platform login query")
	}
	return g.BrowserOrigin() + "/login?" + url.Values{"return": {callback}}.Encode(), nil
}
