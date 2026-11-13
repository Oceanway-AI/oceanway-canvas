package assets

import "testing"

func TestCollectDocumentResourceReferencesIncludesNestedVideoPreview(t *testing.T) {
	raw := `{"nodes":[{"id":"node-video","metadata":{"videoPreview":{"storageKey":"resource:poster-1","source":{"id":"nested-not-node","url":"/api/resources/poster-2/file"}}}}]}`
	refs, err := CollectDocumentResourceReferences(raw)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]DocumentResourceReference{}
	for _, ref := range refs {
		got[ref.ResourceID] = ref
	}
	if ref := got["poster-1"]; ref.Path != "nodes[node-video].metadata.videoPreview.storageKey" || ref.NodeID != "node-video" || ref.ReferenceType != "storageKey" {
		t.Fatalf("nested storageKey reference = %#v", ref)
	}
	if ref := got["poster-2"]; ref.Path != "nodes[node-video].metadata.videoPreview.source.url" || ref.NodeID != "node-video" {
		t.Fatalf("nested URL reference = %#v", ref)
	}
}

func TestCollectDocumentResourceReferencesUsesExplicitFieldsOnly(t *testing.T) {
	raw := `{"nodes":[{"id":"node-1","metadata":{"label":"resource:not-a-reference","videoPreview":{"resourceId":"poster-3"}}}],"log":"/api/resources/log-1/file"}`
	refs, err := CollectDocumentResourceReferences(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ResourceID != "poster-3" || refs[0].NodeID != "node-1" || refs[0].ReferenceType != "resourceId" {
		t.Fatalf("references = %#v", refs)
	}
}

func TestCollectDocumentResourceReferencesReportsInvalidJSON(t *testing.T) {
	if _, err := CollectDocumentResourceReferences(`{"nodes":`); err == nil {
		t.Fatal("expected malformed document to be rejected")
	}
}

func TestCollectDocumentResourceReferencesRetainsCreationResults(t *testing.T) {
	raw := `{"chatMessages":[{"detail":{"resultStorageKeys":["resource:result-image","resource:result-video"],"resultUrls":["/api/resources/old-result/file","https://cdn.test/external"]}}],"prompt":"resource:ignored","log":"/api/resources/ignored/file"}`
	got := map[string]struct{}{}
	if err := CollectOwnedDocumentReferences(raw, got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("result references = %v", got)
	}
	for _, id := range []string{"result-image", "result-video", "old-result"} {
		if _, ok := got[id]; !ok {
			t.Errorf("lost creation result %s", id)
		}
	}
}

func TestCollectDocumentResourceReferencesFromStructuredToolMessages(t *testing.T) {
	raw := `{"messages":[{"role":"tool","tool_call_id":"read","content":"{\"storageKey\":\"resource:tool-file\"}"},{"role":"toolResult","toolCallId":"read-pi","content":[{"type":"text","text":"{\"resourceId\":\"pi-tool-file\"}"}]},{"role":"user","content":"{\"storageKey\":\"resource:prompt-only\"}"},{"role":"tool","content":"tool says resource:diagnostic-only"}],"log":"{\"resourceId\":\"log-only\"}"}`
	got := map[string]struct{}{}
	if err := CollectOwnedDocumentReferences(raw, got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("structured tool references = %v", got)
	}
	for _, id := range []string{"tool-file", "pi-tool-file"} {
		if _, ok := got[id]; !ok {
			t.Errorf("lost structured tool result %s", id)
		}
	}
}
