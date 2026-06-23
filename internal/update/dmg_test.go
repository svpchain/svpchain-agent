//go:build darwin

package update

import (
	"os"
	"testing"
)

func TestStageAppFromReleaseDMG(t *testing.T) {
	dmgPath := os.Getenv("TEST_MACOS_DMG")
	if dmgPath == "" {
		dmgPath = "/tmp/update-test/app.dmg"
	}
	if _, err := os.Stat(dmgPath); err != nil {
		t.Skip("set TEST_MACOS_DMG or place dmg at /tmp/update-test/app.dmg")
	}

	dir := t.TempDir()
	app, err := stageReleasePackage(dmgPath, dir, nil)
	if err != nil {
		t.Fatalf("stageReleasePackage: %v", err)
	}
	if _, err := os.Stat(app); err != nil {
		t.Fatalf("staged app missing: %v", err)
	}
	t.Log("staged", app)
}
