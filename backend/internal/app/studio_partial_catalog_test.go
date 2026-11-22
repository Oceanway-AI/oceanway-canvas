package app

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func capturedProductReceipt(t *testing.T, name string) []byte {
	t.Helper()
	file, err := os.Open(filepath.Join("testdata", "platform-partial-catalog-20261122", name+".json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	z, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	b, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func capturedProductDetail(t *testing.T, name string) ([]byte, platformclient.ModelDetail) {
	t.Helper()
	raw := capturedProductReceipt(t, name)
	var receipt struct {
		Data platformclient.ModelDetail `json:"data"`
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	detail := receipt.Data
	if detail.ModelName != "aggregate-image" || detail.PriceVersion == "" || detail.CapabilityRevision == "" || detail.ConnectionRevision <= 0 {
		t.Fatal("product fixture is missing its published model or revisions")
	}
	return raw, detail
}

// Replay a product response through the installed SDK and the host's normal entry point.
func capturedProductHost(t *testing.T, raw []byte, query platformclient.ModelDetailQuery) (*PublicChannelModel, *platformclient.Response, error) {
	t.Helper()
	session := platformclient.SessionContext{Host: "studio.example.test", Origin: "https://studio.example.test", Cookie: "session=local-fixture"}
	wantQuery := url.Values{
		"model_name": {query.ModelName}, "expected_price_version": {query.ExpectedPriceVersion},
		"expected_capability_revision": {query.ExpectedCapabilityRevision},
		"expected_connection_revision": {strconv.FormatInt(query.ExpectedConnectionRevision, 10)},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/platform/v1/products/studio/model-catalog/detail" || r.URL.Query().Encode() != wantQuery.Encode() {
			t.Errorf("unexpected Studio detail request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Header.Get("Cookie") != session.Cookie || r.Header.Get("X-Platform-Product-Key") != "fixture-studio" {
			t.Error("missing fixture session or Studio product context")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	cfg := map[string]string{
		"STUDIO_PLATFORM_ORIGIN": server.URL, "STUDIO_PLATFORM_PRODUCT_KEY": "fixture-studio",
		"STUDIO_PUBLIC_ORIGIN": session.Origin, "STUDIO_PLATFORM_SESSION_HOSTS": session.Host, "STUDIO_PLATFORM_ALLOWED_ORIGINS": session.Origin,
	}
	gateway, err := platformintegration.NewGatewayFromEnv(func(name string) string { return cfg[name] })
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{studioPlatform: gateway}
	return svc.StudioModelCatalogDetail(t.Context(), session, query, nil)
}

func capturedProductImage(t *testing.T, name string) platformintegration.ModelSelection {
	t.Helper()
	raw, detail := capturedProductDetail(t, name)
	query := platformclient.ModelDetailQuery{ModelName: detail.ModelName, ExpectedPriceVersion: detail.PriceVersion, ExpectedCapabilityRevision: detail.CapabilityRevision, ExpectedConnectionRevision: detail.ConnectionRevision}
	host, response, err := capturedProductHost(t, raw, query)
	if err != nil {
		t.Fatal(err)
	}
	if !host.Available || host.ModelKey != detail.ModelName || response.StatusCode != http.StatusOK || !bytes.Equal(response.Body, raw) {
		t.Fatalf("Studio lost the published model or response: %+v", host)
	}
	var public struct {
		platformclient.Model
		State              string `json:"state"`
		PriceVersion       string `json:"price_version"`
		CapabilityRevision string `json:"capability_revision"`
		ConnectionRevision int64  `json:"connection_revision"`
	}
	if err := json.Unmarshal(host.Platform, &public); err != nil {
		t.Fatal(err)
	}
	if public.State != "detail" || public.PriceVersion != detail.PriceVersion || public.CapabilityRevision != detail.CapabilityRevision || public.ConnectionRevision != detail.ConnectionRevision {
		t.Fatal("Studio projected different selection revisions")
	}
	projected, err := json.Marshal(public.MediaGenerationProfile)
	original, originalErr := json.Marshal(detail.Model.MediaGenerationProfile)
	if err != nil || originalErr != nil || !bytes.Equal(projected, original) {
		t.Fatal("the SDK or host changed the public media profile")
	}
	selected, err := platformintegration.SelectModel(public.Model, "")
	if err != nil || selected.NativeRequest == nil || selected.NativeRequest.ID != "openai-image-generate" || selected.MediaRoute != platformclient.ImageGeneration {
		t.Fatalf("host could not select the published image generation endpoint: %v", err)
	}
	return selected
}

func TestStudioPartialSupplyProductDetail(t *testing.T) {
	type tuple struct {
		size, quality string
		n             int
		pixels        int64
	}
	choices := []tuple{{"1024x1024", "medium", 1, 1048576}, {"1536x1024", "high", 2, 1572864}}
	for _, receipt := range []string{"", "before/"} {
		for index, choice := range choices {
			name := fmt.Sprintf("%sstudio-detail-partial-%d", receipt, index)
			t.Run(name, func(t *testing.T) {
				selected := capturedProductImage(t, name)
				options, err := selected.NativeRequest.Alternatives()
				if err != nil || len(options) != 1 || options[0].OutputSize == nil || options[0].OutputSize.MaxPixels != choice.pixels {
					t.Fatalf("wrong supply alternative in product projection: %+v %v", options, err)
				}
				other := choices[1-index]
				verifyProductDrafts(t, selected, []productDraftCase{
					{"fresh-draft", nil, false, choice.size},
					{"complete", map[string]any{"/size": choice.size, "/quality": choice.quality, "/n": choice.n}, false, choice.size},
					{"only-size", map[string]any{"/size": choice.size}, false, choice.size},
					{"only-quality", map[string]any{"/quality": choice.quality}, false, choice.size},
					{"only-count", map[string]any{"/n": choice.n}, false, choice.size},
					{"other-size", map[string]any{"/size": other.size}, false, ""},
					{"other-quality", map[string]any{"/size": choice.size, "/quality": other.quality}, false, ""},
					{"other-count", map[string]any{"/size": choice.size, "/n": other.n}, false, ""},
					{"reference", map[string]any{"/size": choice.size}, true, ""},
				})
			})
		}
	}
}

func TestStudioCompleteSupplyProductDetail(t *testing.T) {
	selected := capturedProductImage(t, "studio-detail-all")
	options, err := selected.NativeRequest.Alternatives()
	if err != nil || len(options) != 2 {
		t.Fatalf("expected both published supplies to be available: %v", err)
	}
	verifyProductDrafts(t, selected, []productDraftCase{
		{"fresh-draft", nil, false, "1024x1024"},
		{"one-output", map[string]any{"/n": 1}, false, "1024x1024"},
		{"two-outputs", map[string]any{"/n": 2}, false, "1536x1024"},
		{"high", map[string]any{"/quality": "high"}, false, "1536x1024"},
		{"medium", map[string]any{"/quality": "medium"}, false, "1024x1024"},
		{"conflicting-explicit-size", map[string]any{"/n": 2, "/size": "1024x1024"}, false, ""},
		{"conflicting-explicit-quality", map[string]any{"/n": 2, "/quality": "medium"}, false, ""},
		{"invalid-explicit-size", map[string]any{"/size": nil}, false, ""},
	})
}

func TestStudioRequiredImageSizeRemainsImplicitUntilSupplySelection(t *testing.T) {
	selected := capturedProductImage(t, "studio-detail-all")
	for _, spec := range []struct {
		schema *json.RawMessage
		size   string
	}{{&selected.Schema, "/size"}, {&selected.NativeRequest.Schema, "size"}} {
		var properties map[string]json.RawMessage
		if err := json.Unmarshal(*spec.schema, &properties); err != nil {
			t.Fatal(err)
		}
		var required []string
		if err := json.Unmarshal(properties["required"], &required); err != nil {
			t.Fatal(err)
		}
		properties["required"], _ = json.Marshal(append(required, spec.size))
		*spec.schema, _ = json.Marshal(properties)
	}
	verifyProductDrafts(t, selected, []productDraftCase{
		{"choose-two", map[string]any{"/n": 2}, false, "1536x1024"},
		{"explicit-conflict", map[string]any{"/n": 2, "/size": "1024x1024"}, false, ""},
	})
}

func TestStudioRevisedProjectionRejectsPreviousSelection(t *testing.T) {
	_, old := capturedProductDetail(t, "before/studio-detail-partial-1")
	raw, current := capturedProductDetail(t, "studio-detail-partial-1")
	query := platformclient.ModelDetailQuery{
		ModelName: current.ModelName, ExpectedPriceVersion: current.PriceVersion,
		ExpectedConnectionRevision: current.ConnectionRevision, ExpectedCapabilityRevision: old.CapabilityRevision,
	}
	host, response, err := capturedProductHost(t, raw, query)
	var conflict *AppError
	if host != nil || response == nil || response.StatusCode != http.StatusOK || !errors.As(err, &conflict) || conflict.Status != http.StatusConflict || conflict.Reason != "catalog_revision_conflict" {
		t.Fatalf("a selection made under the old projection did not require a refresh: %v", err)
	}
}

type productDraftCase struct {
	name      string
	settings  map[string]any
	reference bool
	wantSize  string
}

func verifyProductDrafts(t *testing.T, selected platformintegration.ModelSelection, drafts []productDraftCase) {
	t.Helper()
	for _, vector := range drafts {
		t.Run(vector.name, func(t *testing.T) {
			input := canvasGenerationInput{}
			input.Config.NativeVariantID = selected.ID
			input.Config.NativeParameters = vector.settings
			values, err := studioNativeSpecifications(input, selected)
			var prepared []byte
			if err == nil {
				var refs []studioNativeReferenceResource
				if vector.reference {
					refs = []studioNativeReferenceResource{{Role: "reference_image", Resource: &model.Resource{Kind: "image", MimeType: "image/png"}, URI: "material://asset/version"}}
				}
				prepared, err = studioNativeResolvedBody("still life", selected, values, refs)
			}
			if (err == nil) != (vector.wantSize != "") {
				t.Fatalf("expected size %q, prepared %s, error %v", vector.wantSize, prepared, err)
			}
			if vector.wantSize == "" {
				return
			}
			want := map[string]any{"model": selected.Name, "prompt": "still life", "size": vector.wantSize}
			for ptr, value := range vector.settings {
				want[ptr[1:]] = value
			}
			expected, err := json.Marshal(want)
			if err != nil || !bytes.Equal(prepared, expected) {
				t.Fatalf("prepared %s, want %s", prepared, expected)
			}
		})
	}
}
