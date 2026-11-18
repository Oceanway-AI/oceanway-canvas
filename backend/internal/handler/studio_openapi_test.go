package handler

import (
	"regexp"
	"strings"
	"testing"
)

func TestStudioOpenAPIRoutesMatchRuntime(t *testing.T) {
	f := newStudioFixture(t)
	RegisterOpenAPIRoutes(f.router.Group("/api"))
	routes := map[string]bool{}
	parameter := regexp.MustCompile(`[:*]([A-Za-z][A-Za-z0-9_]*)`)
	for _, route := range f.router.Routes() {
		path := parameter.ReplaceAllString(strings.TrimPrefix(route.Path, "/api"), "{$1}")
		routes[strings.ToLower(route.Method)+" "+path] = true
	}
	pathPattern := regexp.MustCompile(`^  (/[^:]+):$`)
	methodPattern := regexp.MustCompile(`^    (get|post|put|patch|delete|head|options):$`)
	path := ""
	documented := map[string]bool{}
	for _, line := range strings.Split(string(openAPISpec), "\n") {
		if match := pathPattern.FindStringSubmatch(line); match != nil {
			path = match[1]
		}
		if match := methodPattern.FindStringSubmatch(line); match != nil && path != "" {
			key := match[1] + " " + path
			documented[key] = true
			if !routes[key] {
				t.Errorf("OpenAPI advertises unavailable route %s", key)
			}
		}
	}
	for key := range routes {
		if !documented[key] {
			t.Errorf("OpenAPI is missing Studio route %s", key)
		}
	}
}
