package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStudioStartupRejectsMissingPlatformBeforeDataInitialization(t *testing.T) {
	t.Setenv("STUDIO_PLATFORM_ORIGIN", "")
	dir := filepath.Join(t.TempDir(), "must-not-be-created")
	t.Setenv("CANVAS_BACKEND_DATA_DIR", dir)
	if err := run(context.Background()); err == nil {
		t.Fatal("started without Platform")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("created data before validating Platform configuration")
	}
}
