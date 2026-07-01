package desktop

import (
	"github.com/svpchain/svpchain-agent/internal/agent/runlog"
)

// AgentRunLogPath returns the local JSONL run log file path.
func (a *App) AgentRunLogPath() string {
	return runlog.LogPath()
}

// AgentRecentRuns returns up to limit most recent assistant run traces.
func (a *App) AgentRecentRuns(limit int) ([]runlog.Run, error) {
	return runlog.ReadRecent(limit)
}
