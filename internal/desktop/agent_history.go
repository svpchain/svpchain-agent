package desktop

import (
	"github.com/svpchain/svpchain-agent/internal/agent/history"
)

// TranscriptLine is one displayable chat message restored from history.
type TranscriptLine struct {
	Role string `json:"role"` // "user" | "assistant"
	Text string `json:"text"`
}

// AgentSessions lists saved conversations, most recent first.
func (a *App) AgentSessions() []history.SessionInfo {
	return history.Shared().List()
}

// AgentCurrentSessionID returns the active conversation id ("" when none).
func (a *App) AgentCurrentSessionID() string {
	sess, ok := history.Shared().Current()
	if !ok {
		return ""
	}
	return sess.ID
}

// AgentNewSession starts a fresh conversation and makes it current.
func (a *App) AgentNewSession(chainID string) (history.SessionInfo, error) {
	return history.Shared().Create(chainID)
}

// AgentSwitchSession makes an existing conversation current.
func (a *App) AgentSwitchSession(id string) error {
	return localized(history.Shared().SetCurrent(id))
}

// AgentDeleteSession removes a conversation and its files.
func (a *App) AgentDeleteSession(id string) error {
	return localized(history.Shared().Delete(id))
}

// AgentTranscript returns user/assistant messages of a session for UI restore
// (tool round-trips are omitted). Empty id means the current session.
func (a *App) AgentTranscript(id string) ([]TranscriptLine, error) {
	store := history.Shared()
	if id == "" {
		sess, ok := store.Current()
		if !ok {
			return nil, nil
		}
		id = sess.ID
	}
	entries, err := store.Entries(id)
	if err != nil {
		return nil, err
	}
	var out []TranscriptLine
	for _, e := range entries {
		switch e.Msg.Role {
		case "user":
			out = append(out, TranscriptLine{Role: "user", Text: e.Msg.Content})
		case "assistant":
			if e.Msg.Content != "" && len(e.Msg.ToolCalls) == 0 {
				out = append(out, TranscriptLine{Role: "assistant", Text: e.Msg.Content})
			}
		}
	}
	return out, nil
}
