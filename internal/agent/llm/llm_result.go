package llm

// Usage is token accounting for one LLM round (when the provider reports it).
type Usage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

// Add merges another usage snapshot into u.
func (u *Usage) Add(other Usage) {
	u.PromptTokens += other.PromptTokens
	u.CompletionTokens += other.CompletionTokens
	if other.TotalTokens > 0 {
		u.TotalTokens += other.TotalTokens
	} else if other.PromptTokens > 0 || other.CompletionTokens > 0 {
		u.TotalTokens += other.PromptTokens + other.CompletionTokens
	}
}

// ChatResult is one successful Chat round.
type ChatResult struct {
	Message   Message
	Usage     Usage
	Model     string
	LatencyMs int64
}

type chatRoundResult struct {
	msg   Message
	usage Usage
	model string
}
