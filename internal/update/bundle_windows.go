//go:build windows

package update

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotInstalledLayout means the process is not running from a packaged release folder.
var ErrNotInstalledLayout = errors.New("not running from a packaged Windows install folder")

// Enabled reports whether in-app update is supported on this platform/runtime.
func Enabled() bool {
	_, err := InstallTarget()
	return err == nil
}

// InstallTarget returns the Windows install directory that in-app updates replace.
func InstallTarget() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	base := strings.ToLower(filepath.Base(exe))
	if base != "svpchain-gui.exe" {
		return "", ErrNotInstalledLayout
	}
	return filepath.Dir(exe), nil
}

// findReleaseFolderInDir returns path/to/svpchain agent under dir (zip extract root).
func findReleaseFolderInDir(dir string) (string, error) {
	direct := filepath.Join(dir, appFolderName)
	if st, err := os.Stat(direct); err == nil && st.IsDir() {
		return direct, nil
	}
	var found string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && filepath.Base(path) == appFolderName {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", ErrNotInstalledLayout
	}
	return found, nil
}
