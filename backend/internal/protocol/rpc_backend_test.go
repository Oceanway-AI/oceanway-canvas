package protocol

import "testing"

func TestRPCBackendCandidatesPreferPlatformTaggedFiles(t *testing.T) {
	darwin := RPCBackendCandidates("backend/provider", "darwin", "arm64")
	if got, want := darwin[0], "backend/provider-darwin-arm64"; got != want {
		t.Fatalf("darwin candidates[0] = %q, want %q (%v)", got, want, darwin)
	}
	windows := RPCBackendCandidates("backend/provider", "windows", "amd64")
	if got, want := windows[0], "backend/provider-windows-amd64.exe"; got != want {
		t.Fatalf("windows candidates[0] = %q, want %q (%v)", got, want, windows)
	}
	if windows[len(windows)-1] != "backend/provider" {
		t.Fatalf("windows fallback = %v", windows)
	}
}

func TestHasAnyRPCBackendAcceptsTaggedArtifacts(t *testing.T) {
	if HasAnyRPCBackend("backend/provider", map[string][]byte{"backend/provider-darwin-arm64": {1}}) != true {
		t.Fatal("tagged darwin provider should be accepted")
	}
	if HasAnyRPCBackend("backend/provider", map[string][]byte{"backend/windows-amd64/provider.exe": {1}}) != true {
		t.Fatal("nested windows provider should be accepted")
	}
	if HasAnyRPCBackend("backend/provider", map[string][]byte{"backend/unrelated": {1}}) {
		t.Fatal("unrelated backend file should not satisfy the payment entry")
	}
}
