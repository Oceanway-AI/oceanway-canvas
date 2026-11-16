package app

import (
	"sync"
	"time"

	"crypto/sha256"
	"fmt"
	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

type studioCatalogScope struct{ user, session, host string }

func studioCatalogScopeFor(user string, session platformclient.SessionContext) studioCatalogScope {
	if user == "" || session.Cookie == "" {
		return studioCatalogScope{}
	}
	return studioCatalogScope{user, fmt.Sprintf("%x", sha256.Sum256([]byte(session.Cookie))), session.Host}
}

type studioMediaCatalogSnapshot struct {
	scope   studioCatalogScope
	expires time.Time
	models  map[string]platformclient.Model
}

// Cache only the published schemas used to compile native media requests.
// Account access, project/material ownership and the consumption quote remain live.
type studioMediaCatalogCache struct {
	mu        sync.Mutex
	snapshots [16]studioMediaCatalogSnapshot
}

func (c *studioMediaCatalogCache) remember(scope studioCatalogScope, items []platformclient.Model) {
	if scope.user == "" {
		return
	}
	models := make(map[string]platformclient.Model)
	size := 0
	for _, m := range items {
		if (m.ModelType != "image" && m.ModelType != "video") || m.InvocationStatus != "available" {
			continue
		}
		size += len(m.Price) + len(m.MediaGenerationProfile) + len(m.ModelName) + 512
		if size > 2<<20 {
			models = nil
			break
		}
		models[m.ModelName] = platformclient.Model{ModelName: m.ModelName, ModelType: m.ModelType, InvocationStatus: m.InvocationStatus, Price: m.Price, MediaGenerationProfile: m.MediaGenerationProfile}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	slot := 0
	for i, snapshot := range c.snapshots {
		if snapshot.scope == scope {
			slot = i
			break
		}
		if snapshot.expires.Before(c.snapshots[slot].expires) {
			slot = i
		}
	}
	c.snapshots[slot] = studioMediaCatalogSnapshot{scope, time.Now().Add(time.Minute), models}
}

func (c *studioMediaCatalogCache) lookup(scope studioCatalogScope, name string) (platformclient.Model, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, snapshot := range c.snapshots {
		if snapshot.scope == scope && time.Now().Before(snapshot.expires) {
			m, ok := snapshot.models[name]
			return m, ok
		}
	}
	return platformclient.Model{}, false
}
