// Package step defines the progress-event vocabulary shared by the agent loop and
// the subpackages that report progress (remote auth, memory bootstrap). It is a leaf
// package so those subpackages can emit steps without importing the top-level agent.
package step

// Kind classifies agent progress events.
type Kind string

const (
	Auth   Kind = "auth"
	Tool   Kind = "tool"
	Think  Kind = "think"
	Answer Kind = "answer"
	Error  Kind = "error"
)

// Step is one progress update for the UI.
type Step struct {
	Kind   Kind   `json:"kind"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}
