package llm

import "testing"

func TestUsage_Add(t *testing.T) {
	var total Usage
	total.Add(Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15})
	total.Add(Usage{PromptTokens: 3, CompletionTokens: 2})
	if total.PromptTokens != 13 || total.CompletionTokens != 7 || total.TotalTokens != 20 {
		t.Fatalf("total = %+v", total)
	}
}
