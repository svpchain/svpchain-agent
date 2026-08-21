package desktop

import (
	"context"
	"strings"
	"time"

	"github.com/svpchain/svpchain-agent/internal/agent/runlog"
	"github.com/svpchain/svpchain-agent/internal/chainrpc"
)

// AgentRunLogPath returns the local JSONL run log file path.
func (a *App) AgentRunLogPath() string {
	return runlog.LogPath()
}

// AgentRecentRuns returns up to limit most recent assistant run traces,
// newest first, for the GUI viewer. limit is clamped (default 100, max 200).
func (a *App) AgentRecentRuns(limit int) ([]runlog.Run, error) {
	return runlog.ReadRecentNewestFirst(runlog.ClampRecentLimit(limit))
}

// AgentDeleteRun removes one assistant run trace by id.
func (a *App) AgentDeleteRun(runID string) error {
	return runlog.Delete(runID)
}

// AgentClearRuns deletes every stored assistant run trace.
func (a *App) AgentClearRuns() error {
	return runlog.DeleteAll()
}

// AgentRecheckRunTxs polls CometBFT RPC for this run's tx hashes and rewrites
// tx_checks / intent_checks. Uses the RPC for the run's chain id (testnet:
// https://rpc-testnet.svpchain.org). Agent Hub URL is not used.
func (a *App) AgentRecheckRunTxs(runID string) (runlog.Run, error) {
	chainID := strings.TrimSpace(a.AgentGetSettings().ChainID)
	if runs, err := runlog.ReadAll(""); err == nil {
		want := strings.TrimSpace(runID)
		for _, run := range runs {
			if run.RunID == want {
				if id := strings.TrimSpace(run.ChainID); id != "" {
					chainID = id
				}
				break
			}
		}
	}
	lookup := chainrpc.Lookup(chainrpc.URLForChain(chainID))
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	return runlog.RecheckTxs(ctx, runID, lookup)
}
