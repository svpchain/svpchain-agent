package update

import (
	"strings"

	"golang.org/x/mod/semver"
)

// PrefKeySkipVersion is the Fyne preferences key for a release the user chose to skip.
const PrefKeySkipVersion = "update_skip_version"

// IsDevVersion reports whether local builds should skip update checks.
func IsDevVersion(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || strings.Contains(v, "dev")
}

func normalizeSemver(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return "v0.0.0"
	}
	if !semver.IsValid("v" + v) {
		return "v0.0.0"
	}
	return "v" + v
}

// IsNewer reports whether latest is strictly newer than current (semver).
func IsNewer(latest, current string) bool {
	l := normalizeSemver(latest)
	c := normalizeSemver(current)
	return semver.Compare(l, c) > 0
}
