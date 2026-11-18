package hostupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadInstallerQuotedEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	contents := "POSTGRES_USER='studio'\nDATABASE_URL='postgresql://studio:fixture@postgres/studio'\nSTUDIO_PLATFORM_PRODUCT_KEY='key.literal$value'\nCANVAS_IMAGE_TAG=2.0.0\nCANVAS_HTTP_PORT=\"18560\"\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := readEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"POSTGRES_USER":               "studio",
		"DATABASE_URL":                "postgresql://studio:fixture@postgres/studio",
		"STUDIO_PLATFORM_PRODUCT_KEY": "key.literal$value",
		"CANVAS_IMAGE_TAG":            "2.0.0",
		"CANVAS_HTTP_PORT":            "18560",
	} {
		if values[key] != want {
			t.Errorf("%s did not preserve its configured value", key)
		}
	}
}
