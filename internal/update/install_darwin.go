//go:build darwin

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const helperScript = `#!/bin/bash
set -euo pipefail
APP_PID="$1"
TARGET_BUNDLE="$2"
STAGED_BUNDLE="$3"
wait "$APP_PID" 2>/dev/null || true
sleep 1
rm -rf "$TARGET_BUNDLE"
ditto "$STAGED_BUNDLE" "$TARGET_BUNDLE"
xattr -cr "$TARGET_BUNDLE" 2>/dev/null || true
open "$TARGET_BUNDLE"
`

func stageReleasePackage(packagePath, stagingDir string, progress Progress) (string, error) {
	return stageAppFromDMG(packagePath, stagingDir, progress)
}

// LaunchReplacer starts a helper that replaces target with staged after this process exits.
func LaunchReplacer(target, staged string) error {
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	helperDir := filepath.Join(cache, "com.svpchain.agent-gui", "update")
	if err := os.MkdirAll(helperDir, 0o755); err != nil {
		return err
	}
	helperPath := filepath.Join(helperDir, "apply-update.sh")
	if err := os.WriteFile(helperPath, []byte(helperScript), 0o755); err != nil {
		return err
	}

	cmd := exec.Command("/bin/bash", helperPath,
		strconv.Itoa(os.Getpid()),
		target,
		staged,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start update helper: %w", err)
	}
	go cmd.Wait()
	return nil
}

func stageAppFromDMG(dmgPath, stagingDir string, progress Progress) (string, error) {
	mountPoint, err := os.MkdirTemp(stagingDir, "dmg-mount-*")
	if err != nil {
		return "", err
	}
	defer func() {
		_ = exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()
		_ = os.RemoveAll(mountPoint)
	}()

	cmd := exec.Command("hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mountPoint, dmgPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("hdiutil attach: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if progress != nil {
		progress(1, 2)
	}

	appSrc, err := findAppBundleInDir(mountPoint)
	if err != nil {
		return "", err
	}

	stagedApp := filepath.Join(stagingDir, filepath.Base(appSrc))
	cmd = exec.Command("ditto", appSrc, stagedApp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("copy app from dmg: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if progress != nil {
		progress(2, 2)
	}
	return stagedApp, nil
}
