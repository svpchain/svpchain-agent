package eval_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/eval"
	"github.com/svpchain/svpchain-agent/internal/prefs"
)

func TestGuardCasesRegression(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0o600))
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)

	cases, err := eval.LoadGuardCases("")
	require.NoError(t, err)
	require.NotEmpty(t, cases)
	fails := eval.ScoreGuardCases(cases)
	require.Empty(t, fails, fails)
}
