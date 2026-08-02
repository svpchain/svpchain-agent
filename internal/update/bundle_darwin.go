//go:build darwin

package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotInAppBundle means the process is not running inside a macOS .app bundle.
var ErrNotInAppBundle = errors.New("not running inside a macOS app bundle")

// Enabled reports whether in-app update is supported on this platform/runtime.
func Enabled() bool {
	_, err := InstallTarget()
	return err == nil
}

// InstallTarget returns the macOS .app bundle path that in-app updates replace.
func InstallTarget() (string, error) {
	return AppBundlePath()
}

// AppBundlePath returns the absolute path to the enclosing .app bundle.
func AppBundlePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}

	dir := exe
	for {
		if strings.HasSuffix(dir, ".app") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", ErrNotInAppBundle
}

// findAppBundleInDir returns path/to/svpchain agent.app under dir (DMG mount or extract root).
func findAppBundleInDir(dir string) (string, error) {
	direct := filepath.Join(dir, appBundleName)
	if st, err := os.Stat(direct); err == nil && st.IsDir() {
		return direct, nil
	}
	var found string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && filepath.Base(path) == appBundleName {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("%q not found under %s", appBundleName, dir)
	}
	return found, nil
}
