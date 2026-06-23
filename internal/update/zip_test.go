//go:build windows

package update

import (
	"os"
	"testing"
)

func TestStageAppFromReleaseZip(t *testing.T) {
	zipPath := os.Getenv("TEST_WINDOWS_ZIP")
	if zipPath == "" {
		zipPath = `C:\tmp\update-test\app.zip`
	}
	if _, err := os.Stat(zipPath); err != nil {
		t.Skip("set TEST_WINDOWS_ZIP or place zip at C:\\tmp\\update-test\\app.zip")
	}

	dir := t.TempDir()
	staged, err := stageReleasePackage(zipPath, dir, nil)
	if err != nil {
		t.Fatalf("stageReleasePackage: %v", err)
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("staged folder missing: %v", err)
	}
	t.Log("staged", staged)
}
