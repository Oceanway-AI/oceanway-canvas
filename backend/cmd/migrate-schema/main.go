package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"yingce/backend/internal/canvas"
	"yingce/backend/internal/database"
	"yingce/backend/internal/platformintegration"
	"yingce/backend/internal/service"
)

func main() {
	command := "up"
	if len(os.Args) > 1 {
		command = strings.ToLower(strings.TrimSpace(os.Args[1]))
	}
	apply := false
	if command == "repair-asset-bytes" || command == "repair-canvas-resources" || command == "reset-studio" {
		if len(os.Args) == 3 && os.Args[2] == "--apply" {
			apply = true
		} else if len(os.Args) > 2 {
			log.Fatalf("用法：migrate-schema %s [--apply]；默认只检查", command)
		}
	}
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		log.Fatal("必须显式设置 PostgreSQL DATABASE_URL")
	}
	db, err := database.Open(database.Config{
		Driver:  env("CANVAS_DATABASE_DRIVER", "postgres"),
		DSN:     os.Getenv("DATABASE_URL"),
		DataDir: env("CANVAS_BACKEND_DATA_DIR", "data"),
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := database.ConfigurePool(db); err != nil {
		log.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()

	switch command {
	case "reset-studio":
		var gateway *platformintegration.Gateway
		if apply {
			gateway, err = platformintegration.NewGatewayFromEnv(os.Getenv)
			if err != nil {
				log.Fatal(err)
			}
		}
		report, resetErr := service.ResetStudioDatabase(context.Background(), db, os.Getenv("CANVAS_BACKEND_DATA_DIR"), gateway, apply)
		encoded, err := json.Marshal(report)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(encoded))
		if resetErr != nil {
			log.Fatal(resetErr)
		}
		return
	case "repair-canvas-resources":
		if err := database.RequireStudioSchemaVersion(db); err != nil {
			log.Fatal(err)
		}
		report, repairErr := canvas.RepairCanvasResources(db, apply)
		encoded, err := json.Marshal(report)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(encoded))
		if repairErr != nil {
			log.Fatal(repairErr)
		}
		if report.Unresolved > 0 {
			log.Fatal("存在需人工处理的画布，请按报告恢复历史或重新上传；未删除主媒体")
		}
		return
	case "repair-asset-bytes":
		report, repairErr := canvas.RepairAssetBytes(db, apply)
		encoded, err := json.Marshal(report)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(encoded))
		if repairErr != nil {
			log.Fatal(repairErr)
		}
		if len(report.Unresolved) > 0 {
			log.Fatal("存在未修复素材，请按报告核查；未删除或填零")
		}
		return
	case "up":
		if err := database.MigrateStudioSchema(db); err != nil {
			log.Fatal(err)
		}
	case "status":
	case "verify":
		if err := database.RequireStudioSchemaVersion(db); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("未知命令 %q；可用命令：up、status、verify、reset-studio、repair-asset-bytes、repair-canvas-resources", command)
	}
	status, err := database.ReadStudioSchemaStatus(db)
	if err != nil {
		log.Fatal(err)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(encoded))
}

func env(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
