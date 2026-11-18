package database

import (
	"os"
	"testing"
)

func TestOpenRequiresPostgresConfiguration(t *testing.T) {
	for _, driver := range []string{"sqlite", "mysql", "", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			dir := t.TempDir()
			if db, err := Open(Config{Driver: driver, DataDir: dir}); err == nil || db != nil {
				t.Fatalf("accepted missing PostgreSQL configuration: db=%v err=%v", db, err)
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 0 {
				t.Fatalf("created a local database: files=%v err=%v", files, err)
			}
		})
	}
}
