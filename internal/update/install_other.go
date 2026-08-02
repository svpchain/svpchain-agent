//go:build !darwin && !windows

package update

func stageReleasePackage(packagePath, stagingDir string, progress Progress) (string, error) {
	return "", ErrUnsupportedPlatform
}

// LaunchReplacer is unavailable on platforms without packaged in-app updates.
func LaunchReplacer(target, staged string) error {
	return ErrUnsupportedPlatform
}
