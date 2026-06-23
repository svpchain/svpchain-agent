//go:build windows

package update

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const helperScript = `$ErrorActionPreference = "Stop"
param(
	[int]$AppPid,
	[string]$TargetDir,
	[string]$StagedDir
)
Wait-Process -Id $AppPid -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1
Get-ChildItem -Path $StagedDir -File | ForEach-Object {
	Copy-Item -Path $_.FullName -Destination (Join-Path $TargetDir $_.Name) -Force
}
Start-Process (Join-Path $TargetDir "svpchain-gui.exe")
`

func stageReleasePackage(packagePath, stagingDir string, progress Progress) (string, error) {
	extractDir := filepath.Join(stagingDir, "extract")
	if err := unzip(packagePath, extractDir, progress); err != nil {
		return "", err
	}

	releaseDir, err := findReleaseFolderInDir(extractDir)
	if err != nil {
		return "", err
	}

	stagedDir := filepath.Join(stagingDir, appFolderName)
	if err := copyDir(releaseDir, stagedDir); err != nil {
		return "", err
	}
	return stagedDir, nil
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
	helperPath := filepath.Join(helperDir, "apply-update.ps1")
	if err := os.WriteFile(helperPath, []byte(helperScript), 0o755); err != nil {
		return err
	}

	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", helperPath,
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

func unzip(src, dest string, progress Progress) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	totalFiles := int64(0)
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			totalFiles++
		}
	}

	var done int64
	for _, f := range r.File {
		name := filepath.FromSlash(f.Name)
		path := filepath.Join(dest, name)
		if !filepath.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid zip entry: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := rc.Close()
		cerr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if cerr != nil {
			return cerr
		}
		done++
		if progress != nil && totalFiles > 0 {
			progress(done, totalFiles)
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}
