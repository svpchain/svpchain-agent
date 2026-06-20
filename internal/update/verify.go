package update

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func expectedHashFromSums(sums []byte, zipName string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(sums))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := parts[len(parts)-1]
		if name == zipName {
			return strings.ToLower(parts[0]), nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS has no entry for %q", zipName)
}

func verifyZipChecksum(zipPath, zipName string, sums []byte) error {
	want, err := expectedHashFromSums(sums, zipName)
	if err != nil {
		return err
	}
	got, err := hashFile(zipPath)
	if err != nil {
		return fmt.Errorf("hash zip: %w", err)
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s", zipName)
	}
	return nil
}
