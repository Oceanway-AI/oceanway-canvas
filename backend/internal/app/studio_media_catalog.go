package app

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

type studioCatalogScope struct{ user, session, host, workspace string }

func studioCatalogScopeFor(user string, session platformclient.SessionContext, verifiedSessionID ...string) studioCatalogScope {
	if user == "" || session.Host == "" || session.Cookie == "" || len(verifiedSessionID) != 1 || verifiedSessionID[0] == "" {
		return studioCatalogScope{}
	}
	// Platform's live Identity has already authenticated this SID. Other
	// browser cookies and a rotated cookie for the same SID are immaterial.
	return studioCatalogScope{user: user, session: fmt.Sprintf("%x", sha256.Sum256([]byte(verifiedSessionID[0]))), host: session.Host}
}

type studioMediaCatalogSnapshot struct {
	scope       studioCatalogScope
	modelName   string
	expires     time.Time
	models      map[string]platformclient.Model
	previewOnly map[string]string
}

// Cache only the published schemas used to compile native media requests.
// Account access, project/material ownership and the consumption quote remain live.
type studioMediaCatalogCache struct {
	mu        sync.Mutex
	snapshots [16]studioMediaCatalogSnapshot
}

func (c *studioMediaCatalogCache) remember(scope studioCatalogScope, items []platformclient.Model) {
	c.rememberModels(scope, items, "")
}

func (c *studioMediaCatalogCache) rememberDetail(scope studioCatalogScope, item platformclient.Model) {
	c.rememberModels(scope, []platformclient.Model{item}, item.ModelName)
}

func (c *studioMediaCatalogCache) rememberModels(scope studioCatalogScope, items []platformclient.Model, modelName string) {
	if scope.user == "" {
		return
	}
	models := make(map[string]platformclient.Model)
	previewOnly := make(map[string]string)
	size := 0
	for _, m := range items {
		if (m.ModelType != "image" && m.ModelType != "video") || m.InvocationStatus != "available" {
			continue
		}
		var profile struct {
			Variants []struct {
				ID      string `json:"id"`
				Enabled bool   `json:"media_generation_v1"`
			} `json:"variants"`
		}
		if json.Unmarshal(m.MediaGenerationProfile, &profile) != nil {
			continue
		}
		enabled := false
		for _, variant := range profile.Variants {
			if variant.ID != "" && variant.Enabled {
				enabled = true
				break
			}
		}
		cost := len(m.Price) + len(m.MediaGenerationProfile) + len(m.ModelName) + 512
		if !enabled {
			// A legacy selection still needs its account-bound confirmation.
			// A newly selected variant or price still gets a fresh catalog.
			var price struct{ Version string }
			if len(profile.Variants) > 0 && len(previewOnly) < 1024 && json.Unmarshal(m.Price, &price) == nil && price.Version != "" {
				previewOnly[m.ModelName] = price.Version
			}
			continue
		}
		if size+cost > 2<<20 {
			continue
		}
		size += cost
		models[m.ModelName] = platformclient.Model{ModelName: m.ModelName, ModelType: m.ModelType, InvocationStatus: m.InvocationStatus, Price: m.Price, MediaGenerationProfile: m.MediaGenerationProfile}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	slot := 0
	for i, snapshot := range c.snapshots {
		if snapshot.scope == scope && snapshot.modelName == modelName {
			slot = i
			break
		}
		if snapshot.expires.Before(c.snapshots[slot].expires) {
			slot = i
		}
	}
	c.snapshots[slot] = studioMediaCatalogSnapshot{scope: scope, modelName: modelName, expires: time.Now().Add(time.Minute), models: models, previewOnly: previewOnly}
}

func (c *studioMediaCatalogCache) requiresPreview(scope studioCatalogScope, req CreateTaskRequest) bool {
	if req.Platform == nil || req.Platform.ExpectedPriceVersion == "" || req.Platform.VariantID != "" || (req.Type != "canvas_image" && req.Type != "canvas_video") {
		return false
	}
	scope.workspace = req.Platform.WorkspaceID
	config, _ := req.Input["config"].(map[string]any)
	if stringValue(config["nativeVariantId"]) != "" {
		return false
	}
	name := firstNonEmpty(stringValue(config["model"]), req.Model)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, snapshot := range c.snapshots {
		if snapshot.scope == scope && (snapshot.modelName == "" || snapshot.modelName == name) && time.Now().Before(snapshot.expires) && snapshot.previewOnly[name] == req.Platform.ExpectedPriceVersion {
			return true
		}
	}
	return false
}

func (c *studioMediaCatalogCache) lookup(scope studioCatalogScope, name string, explicitlyVersioned bool) (platformclient.Model, bool) {
	c.mu.Lock()
	for _, snapshot := range c.snapshots {
		if snapshot.scope == scope && snapshot.modelName == name && (explicitlyVersioned || time.Now().Before(snapshot.expires)) {
			m, ok := snapshot.models[name]
			c.mu.Unlock()
			return m, ok
		}
	}
	c.mu.Unlock()
	m, ok := c.snapshot(scope, explicitlyVersioned).models[name]
	return m, ok
}

func (c *studioMediaCatalogCache) snapshot(scope studioCatalogScope, explicitlyVersioned bool) studioMediaCatalogSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, snapshot := range c.snapshots {
		if snapshot.scope == scope && snapshot.modelName == "" && (explicitlyVersioned || time.Now().Before(snapshot.expires)) {
			return snapshot
		}
	}
	return studioMediaCatalogSnapshot{}
}
