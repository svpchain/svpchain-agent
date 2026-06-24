package signer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/svpchain/svpchain-agent/internal/prefs"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "svpchain-signer-prefs")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "prefs.json")
	_ = os.WriteFile(path, []byte(`{"whitelist":[]}`), 0o600)
	prefs.SetPathOverride(path)
	code := m.Run()
	prefs.SetPathOverride("")
	os.Exit(code)
}
