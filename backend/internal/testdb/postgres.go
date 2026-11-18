// Package testdb gives database tests an isolated PostgreSQL schema.
package testdb

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(t testing.TB, options ...gorm.Option) (*gorm.DB, error) {
	t.Helper()
	dsn := os.Getenv("CANVAS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("CANVAS_TEST_POSTGRES_DSN is required for PostgreSQL tests")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	schema := "test_" + uuid.NewString()[:8] + fmt.Sprintf("_%d", time.Now().UnixNano())
	base, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	baseSQL, err := base.DB()
	if err != nil {
		return nil, err
	}
	if err := base.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		_ = baseSQL.Close()
		return nil, err
	}
	t.Cleanup(func() {
		// PostgreSQL retains DDL locks until commit. Serialize large schema
		// drops across test processes to keep the shared lock table bounded.
		err := base.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(73123910420261118)).Error; err != nil {
				return err
			}
			return tx.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
		})
		if err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		_ = baseSQL.Close()
	})
	config.RuntimeParams["search_path"] = schema
	return connect(t, config, options...)
}

// OpenDatabase isolates tests of database-wide statistics from concurrent
// schema fixtures. The test role must have CREATEDB, as in the CI service.
func OpenDatabase(t testing.TB, options ...gorm.Option) (*gorm.DB, error) {
	t.Helper()
	dsn := os.Getenv("CANVAS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("CANVAS_TEST_POSTGRES_DSN is required for PostgreSQL tests")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	admin := stdlib.OpenDB(*config)
	name := "test_" + uuid.NewString()
	quotedName := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec("CREATE DATABASE " + quotedName + " TEMPLATE template0"); err != nil {
		_ = admin.Close()
		return nil, err
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP DATABASE " + quotedName); err != nil {
			t.Errorf("drop test database: %v", err)
		}
		_ = admin.Close()
	})
	config.Database = name
	return connect(t, config, options...)
}

// Reopen creates a separate pool against the same isolated schema for
// cross-connection transaction and concurrency tests.
func Reopen(t testing.TB, source *gorm.DB, options ...gorm.Option) (*gorm.DB, error) {
	t.Helper()
	var schema string
	if err := source.Raw("SELECT current_schema()").Scan(&schema).Error; err != nil {
		return nil, err
	}
	config, err := pgx.ParseConfig(os.Getenv("CANVAS_TEST_POSTGRES_DSN"))
	if err != nil {
		return nil, err
	}
	config.RuntimeParams["search_path"] = schema
	return connect(t, config, options...)
}

func connect(t testing.TB, config *pgx.ConnConfig, options ...gorm.Option) (*gorm.DB, error) {
	sqlDB := stdlib.OpenDB(*config)
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(2)
	t.Cleanup(func() { _ = sqlDB.Close() })
	options = append([]gorm.Option{&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}}, options...)
	return gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), options...)
}
