package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnzipReleaseMacOSZip(t *testing.T) {
	zipPath := os.Getenv("TEST_MACOS_ZIP")
	if zipPath == "" {
		zipPath = "/tmp/update-test/zip.zip"
	}
	if _, err := os.Stat(zipPath); err != nil {
		t.Skip("set TEST_MACOS_ZIP or place zip at /tmp/update-test/zip.zip")
	}

	dir := t.TempDir()
	extract := filepath.Join(dir, "extract")
	if err := unzip(zipPath, extract, nil); err != nil {
		t.Fatalf("unzip: %v", err)
	}
	app, err := findAppBundleInDir(extract)
	if err != nil {
		t.Fatalf("findAppBundleInDir: %v", err)
	}
	t.Log("found", app)
}
