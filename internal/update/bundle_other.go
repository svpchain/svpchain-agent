//go:build !darwin && !windows

package update

import "errors"

// ErrUnsupportedPlatform means in-app update is not available on this OS.
var ErrUnsupportedPlatform = errors.New("in-app update not supported on this platform")

// Enabled reports whether in-app update is supported on this platform/runtime.
func Enabled() bool { return false }

// InstallTarget is unavailable on platforms without packaged in-app updates.
func InstallTarget() (string, error) {
	return "", ErrUnsupportedPlatform
}
