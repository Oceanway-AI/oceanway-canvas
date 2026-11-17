// Package catalogtest migrates existing synthetic model fixtures to the
// versioned catalog wire for consumer tests. Production code never imports it.
package catalogtest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

func Route(path string) bool {
	return strings.HasSuffix(path, "/models") || strings.HasSuffix(path, "/model-catalog/summaries") || strings.HasSuffix(path, "/model-catalog/detail")
}

// Write keeps the old fixture's exact model data. Older synthetic profiles
// share the a/7 capability revisions already used by their quote fixtures.
func Write(t testing.TB, w http.ResponseWriter, r *http.Request, source []byte) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if strings.HasSuffix(r.URL.Path, "/models") {
		w.Write(source)
		return
	}
	var catalog struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(source, &catalog); err != nil {
		t.Error(err)
		w.WriteHeader(500)
		return
	}
	revision := fmt.Sprintf("%x", sha256.Sum256(source))
	summaries := []platformclient.ModelSummary{}
	for _, raw := range catalog.Data.Items {
		var item platformclient.Model
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Error(err)
			return
		}
		var price struct {
			Version string `json:"version"`
		}
		json.Unmarshal(item.Price, &price)
		var profile struct {
			Revision   string `json:"revision"`
			Connection int64  `json:"connection_revision"`
			Default    string `json:"default_variant"`
			Variants   []struct {
				ID         string `json:"id"`
				Protocol   string `json:"protocol"`
				Generation bool   `json:"media_generation_v1"`
				Native     *struct {
					References []platformclient.ModelReferenceSummary `json:"native_reference_inputs"`
				} `json:"native_request_variant"`
			} `json:"variants"`
		}
		json.Unmarshal(item.MediaGenerationProfile, &profile)
		if item.ModelType != "text" && profile.Revision == "" {
			profile.Revision, profile.Connection = strings.Repeat("a", 64), 7
		}
		summary := platformclient.ModelSummary{Model: item, State: "summary", PriceVersion: price.Version, CapabilityRevision: profile.Revision, ConnectionRevision: profile.Connection, DefaultVariant: profile.Default, VariantsComplete: true, Variants: []platformclient.ModelVariantSummary{}}
		for _, variant := range profile.Variants {
			view := platformclient.ModelVariantSummary{ID: variant.ID, Protocol: variant.Protocol, MediaGenerationV1: variant.Generation, ReferencesComplete: false, RequiresDetail: true}
			if variant.Native != nil {
				view.References, view.ReferencesComplete = variant.Native.References, true
			}
			summary.Variants = append(summary.Variants, view)
		}
		if item.ModelType != "text" && len(summary.Variants) == 0 {
			summary.Variants = []platformclient.ModelVariantSummary{{RequiresDetail: true}}
		}
		if strings.HasSuffix(r.URL.Path, "/detail") {
			query := r.URL.Query()
			if query.Get("model_name") != item.ModelName {
				continue
			}
			if query.Get("expected_price_version") != "" && query.Get("expected_price_version") != price.Version || query.Get("expected_capability_revision") != "" && query.Get("expected_capability_revision") != profile.Revision || query.Get("expected_connection_revision") != "" && query.Get("expected_connection_revision") != strconv.FormatInt(profile.Connection, 10) {
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]any{"success": false, "reason": "catalog_revision_conflict"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"version": platformclient.ModelCatalogVersion, "state": "detail", "catalog_revision": revision, "model_name": item.ModelName, "variant_id": query.Get("variant_id"), "price_version": price.Version, "capability_revision": profile.Revision, "connection_revision": profile.Connection, "model": json.RawMessage(raw)}})
			return
		}
		if query := r.URL.Query().Get("query"); query != "" && !strings.Contains(item.ModelName, query) {
			continue
		}
		summary.MediaGenerationProfile = nil
		summaries = append(summaries, summary)
	}
	if strings.HasSuffix(r.URL.Path, "/detail") {
		w.WriteHeader(404)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"success": true, "data": platformclient.ModelSummaryPage{Version: platformclient.ModelCatalogVersion, State: "summary", CatalogRevision: revision, Items: summaries}})
}
