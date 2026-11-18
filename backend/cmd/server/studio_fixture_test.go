package main

import (
	"testing"

	"yingce/backend/internal/database"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
	"yingce/backend/internal/service"
	"yingce/backend/internal/testdb"

	"gorm.io/gorm"
)

func newStudioTestService(t *testing.T, db *gorm.DB) *service.Service {
	t.Helper()
	testdb.UseRedis(t)
	if err := database.MigrateStudioSchema(db); err != nil {
		t.Fatal(err)
	}
	config := map[string]string{
		"STUDIO_PLATFORM_ORIGIN":          "https://platform.example.test",
		"STUDIO_PLATFORM_PRODUCT_KEY":     "fixture.secret",
		"STUDIO_PUBLIC_ORIGIN":            "https://studio.example.test",
		"STUDIO_PLATFORM_SESSION_HOSTS":   "studio.example.test",
		"STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test",
	}
	gateway, err := platformintegration.NewGatewayFromEnv(func(key string) string { return config[key] })
	if err != nil {
		t.Fatal(err)
	}
	svc, err := service.NewStudio(repository.NewStudio(db), t.TempDir(), gateway)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}
