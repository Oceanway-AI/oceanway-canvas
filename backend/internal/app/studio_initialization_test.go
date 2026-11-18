package app

import (
	"context"
	"testing"
	"time"
	"yingce/backend/internal/testdb"

	"yingce/backend/internal/database"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
)

func TestStudioInitializationWithoutLocalPlatformServices(t *testing.T) {
	testdb.UseRedis(t)
	dir := t.TempDir()
	db, err := testdb.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	env := map[string]string{
		"STUDIO_PLATFORM_ORIGIN":          "https://platform.example.test",
		"STUDIO_PLATFORM_PRODUCT_KEY":     "fixture.secret",
		"STUDIO_PUBLIC_ORIGIN":            "https://studio.example.test",
		"STUDIO_PLATFORM_SESSION_HOSTS":   "studio.example.test",
		"STUDIO_PLATFORM_ALLOWED_ORIGINS": "https://studio.example.test",
	}
	gateway, err := platformintegration.NewGatewayFromEnv(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewStudio(repository.NewStudio(db), dir, gateway); err == nil {
		t.Fatal("started without Studio schema")
	}
	if err := database.MigrateStudioSchema(db); err != nil {
		t.Fatal(err)
	}
	svc, err := NewStudio(repository.NewStudio(db), dir, gateway)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if err := svc.ValidateRuntime(); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func() error{svc.EnsureDefaultPromptTemplates, svc.EnsureBuiltinProjectWorkflowTemplate, svc.EnsureBuiltinSkills, svc.EnsureSkillPackages, svc.EnsureBuiltinTools} {
		if err := initialize(); err != nil {
			t.Fatal(err)
		}
	}
	if svc.skills == nil || svc.canvas == nil || svc.taskWorkerCoordinator == nil || svc.taskLifecycleCoordinator == nil {
		t.Fatal("creative services missing")
	}
	checkRetired := func() {
		t.Helper()
		if svc.pluginRuntime != nil || svc.taskBillingCoordinator != nil || svc.taskTerminalCoordinator != nil || svc.taskRouteExecutor != nil {
			t.Fatal("Studio initialized retired services")
		}
	}
	checkRetired()
	svc.StartStudioWorker()
	svc.StartStudioWorker()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.StopWorker(ctx); err != nil {
		t.Fatal(err)
	}
	checkRetired()
}
