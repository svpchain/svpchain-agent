package settlement

import "strings"

// ActiveTask is a funded and assigned settlement task that has not yet been
// reported complete.
//
// It exists because payment is per behavior, not per message: a behavior the
// user asked for in one turn routinely finishes in a later one — the assistant
// asks a clarifying question, or the work needs a second transaction. The task
// is carried across those turns so answering a question does not buy a second
// escrow, and is dropped once its execution has been reported, which is what
// makes the next behavior pay again.
//
// Amount and IntentID are carried for audit only. Reuse needs TaskID and Owner;
// the deposit they name has already moved and is never re-funded from here.
type ActiveTask struct {
	Endpoint string `json:"endpoint"`
	AgentID  string `json:"agent_id,omitempty"`
	TaskID   string `json:"task_id"`
	IntentID string `json:"intent_id,omitempty"`
	Owner    string `json:"owner"`
	Amount   string `json:"amount,omitempty"`
}

// Usable reports whether this record identifies a task that can be reused: an
// endpoint to match it against, and the two fields the reporter needs to close
// it out. A partial record is treated as no record, so a run funds rather than
// silently reusing something it cannot report against.
func (t *ActiveTask) Usable() bool {
	return t != nil &&
		strings.TrimSpace(t.Endpoint) != "" &&
		normalizeHash(t.TaskID) != "" &&
		strings.TrimSpace(t.Owner) != ""
}
