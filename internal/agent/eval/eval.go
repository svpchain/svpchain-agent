package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/svpchain/svpchain-agent/internal/agent/guard"
	"github.com/svpchain/svpchain-agent/internal/agent/runlog"
)

// GuardCase is one offline regression case for the transfer whitelist gate.
type GuardCase struct {
	ID            string         `json:"id"`
	Description   string         `json:"description"`
	ChainID       string         `json:"chain_id"`
	Tool          string         `json:"tool"`
	Args          map[string]any `json:"args"`
	ExpectOutcome runlog.Outcome `json:"expect_outcome"`
}

// LoadGuardCases reads guard regression cases from path (or the bundled default).
func LoadGuardCases(path string) ([]GuardCase, error) {
	if path == "" {
		path = defaultGuardCasesPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []GuardCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func defaultGuardCasesPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "testdata/agent_eval/guard_cases.json"
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "agent_eval", "guard_cases.json"))
}

// RunGuardCase executes one guard case and returns the observed outcome.
func RunGuardCase(c GuardCase) runlog.Outcome {
	err := guard.Check(c.ChainID, c.Tool, c.Args)
	return runlog.ClassifyGuardError(err)
}

// ScoreGuardCases runs all cases and returns failures.
func ScoreGuardCases(cases []GuardCase) []string {
	var fails []string
	for _, c := range cases {
		got := RunGuardCase(c)
		if got != c.ExpectOutcome {
			fails = append(fails, fmt.Sprintf("%s: want %s got %s (%s)", c.ID, c.ExpectOutcome, got, c.Description))
		}
	}
	return fails
}
