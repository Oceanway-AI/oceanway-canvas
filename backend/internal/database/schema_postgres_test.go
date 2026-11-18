package database

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

func TestPostgresStudioAssetIDs(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("CANVAS_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("CANVAS_TEST_POSTGRES_DSN is not configured")
	}

	base, err := Open(Config{Driver: "postgres", DSN: dsn})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	baseSQL, err := base.DB()
	if err != nil {
		t.Fatalf("postgres sql db: %v", err)
	}
	defer baseSQL.Close()

	schemaName := fmt.Sprintf("asset_id_migration_%d", time.Now().UnixNano())
	if err := base.Exec(`CREATE SCHEMA "` + schemaName + `"`).Error; err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	defer func() {
		if err := base.Exec(`DROP SCHEMA IF EXISTS "` + schemaName + `" CASCADE`).Error; err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	}()

	testDSN, err := postgresDSNWithSearchPath(dsn, schemaName)
	if err != nil {
		t.Fatalf("test postgres dsn: %v", err)
	}
	db, err := Open(Config{Driver: "postgres", DSN: testDSN})
	if err != nil {
		t.Fatalf("open test schema: %v", err)
	}
	dbSQL, err := db.DB()
	if err != nil {
		t.Fatalf("test schema sql db: %v", err)
	}
	defer dbSQL.Close()

	if err := MigrateStudioSchema(db); err != nil {
		t.Fatal(err)
	}

	assetID := "generation_" + strings.Repeat("a", 64)
	if err := db.Create(&model.Asset{ID: assetID, UserID: "user-1", Kind: "image"}).Error; err != nil {
		t.Fatalf("insert deterministic generation asset: %v", err)
	}
	if err := db.Create(&model.ProjectAssetLink{ID: "link-1", ProjectID: "project-1", AssetID: assetID}).Error; err != nil {
		t.Fatalf("insert project asset link: %v", err)
	}
	if err := db.Create(&model.ProjectAssetCandidate{ID: "candidate-1", ProjectID: "project-1", ResolvedAssetID: assetID}).Error; err != nil {
		t.Fatalf("insert project asset candidate: %v", err)
	}
	if err := db.Create(&model.AssetVersion{ID: "version-1", AssetID: assetID, Version: 1}).Error; err != nil {
		t.Fatalf("insert asset version: %v", err)
	}
}

func postgresDSNWithSearchPath(dsn string, schemaName string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
