// Administrative maintenance only. This executable never starts generation workers.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"yingce/backend/internal/database"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/repository"
	"yingce/backend/internal/service"
)

func main() {
	sync := flag.Bool("sync", false, "sync the pinned, license-checked source allowlist")
	bootstrap := flag.Bool("bootstrap-builtin", false, "insert or upgrade shipped KB01-KB10 entries; local edits and review status are preserved")
	bundled := flag.Bool("bootstrap-bundled", false, "initialize the offline knowledge snapshot without overwriting existing records")
	sessionFile := flag.String("session-file", "", "private file containing the current Platform session Cookie, required for writes")
	flag.Parse()
	if os.Getenv("DATABASE_URL") == "" {
		log.Fatal("explicit database location required")
	}
	driver := os.Getenv("CANVAS_DATABASE_DRIVER")
	if driver == "" {
		driver = "postgres"
	}
	dir := os.Getenv("CANVAS_BACKEND_DATA_DIR")
	db, err := database.Open(database.Config{Driver: driver, DSN: os.Getenv("DATABASE_URL"), DataDir: dir})
	if err != nil {
		log.Fatal(err)
	}
	if err := database.RequireStudioSchemaVersion(db); err != nil {
		log.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()
	gateway, err := platformintegration.NewGatewayFromEnv(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	svc, err := service.NewStudio(repository.NewStudio(db), dir, gateway)
	if err != nil {
		log.Fatal(err)
	}
	defer svc.Close()
	var actor *model.User
	if *bootstrap || *bundled || *sync {
		cookie, readErr := os.ReadFile(*sessionFile)
		if readErr != nil || strings.TrimSpace(string(cookie)) == "" {
			log.Fatal("a private Platform session file is required for knowledge writes")
		}
		request, requestErr := http.NewRequest(http.MethodGet, os.Getenv("STUDIO_PUBLIC_ORIGIN"), nil)
		if requestErr != nil {
			log.Fatal("invalid Studio public origin")
		}
		request.Header.Set("Origin", os.Getenv("STUDIO_PUBLIC_ORIGIN"))
		request.Header.Set("Cookie", strings.TrimSpace(string(cookie)))
		actor, _, err = svc.StudioCurrentUser(context.Background(), platformintegration.Session(request))
		if err != nil || svc.RequireAdmin(actor) != nil {
			log.Fatal("current Platform session lacks Studio content management permission")
		}
	}
	if *bundled {
		report, err := svc.EnsureBundledEcommerceKnowledge()
		if err != nil {
			log.Fatal(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"bundle": report}); err != nil {
			log.Fatal(err)
		}
	}
	if *bootstrap {
		report, err := svc.EnsureBuiltinEcommerceKnowledge()
		if err != nil {
			log.Fatal(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"bootstrap": report}); err != nil {
			log.Fatal(err)
		}
		industry, err := svc.EnsureIndustryTaxonomyKnowledge()
		if err != nil {
			log.Fatal(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"industryTaxonomy": industry}); err != nil {
			log.Fatal(err)
		}
	}
	if *sync {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		result, err := svc.SyncEcommerceKnowledge(ctx, actor)
		if err != nil {
			log.Fatal(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			log.Fatal(err)
		}
	}
	stats, err := svc.EcommerceKnowledgeStats()
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(stats); err != nil {
		log.Fatal(err)
	}
}
