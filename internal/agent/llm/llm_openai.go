package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type openAIChatRequest struct {
	Model         string               `json:"model"`
	Messages      []Message            `json:"messages"`
	Tools         []Tool               `json:"tools,omitempty"`
	Stream        bool                 `json:"stream"`
	StreamOptions *openAIStreamOptions `json:"stream_options,omitempty"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// openAIStreamChunk is one SSE `data:` frame from /v1/chat/completions (stream=true).
type openAIStreamChunk struct {
	Choices []openAIStreamChoice `json:"choices"`
	Usage   *openAIUsage         `json:"usage,omitempty"`
	Model   string               `json:"model,omitempty"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIStreamChoice struct {
	Delta openAIStreamDelta `json:"delta"`
}

type openAIStreamDelta struct {
	Content   string                 `json:"content"`
	ToolCalls []openAIStreamToolCall `json:"tool_calls"`
}

type openAIStreamToolCall struct {
	Index    int                      `json:"index"`
	ID       string                   `json:"id"`
	Function openAIStreamToolCallFunc `json:"function"`
}

type openAIStreamToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// openAIToolCallAcc accumulates one tool call sharded across stream frames.
type openAIToolCallAcc struct {
	id   string
	name string
	args strings.Builder
}

type openaiModel struct {
	cfg    Config
	client *http.Client
}

func (m *openaiModel) Chat(ctx context.Context, messages []Message, tools []Tool, emit func(string)) (ChatResult, error) {
	if emit == nil {
		emit = func(string) {}
	}
	body, err := json.Marshal(openAIChatRequest{
		Model:         m.cfg.Model,
		Messages:      messages,
		Tools:         tools,
		Stream:        true,
		StreamOptions: &openAIStreamOptions{IncludeUsage: true},
	})
	if err != nil {
		return ChatResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)
	resp, err := m.client.Do(req)
	if err != nil {
		return ChatResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ChatResult{}, httpError(resp)
	}

	out := Message{Role: "assistant"}
	var usage Usage
	var respModel string
	var contentB strings.Builder
	// tool_calls arrive sharded by index across frames; accumulate per index.
	calls := map[int]*openAIToolCallAcc{}
	var order []int

	err = scanSSE(resp.Body, func(data string) (bool, error) {
		if data == "[DONE]" {
			return true, nil
		}
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return false, nil // tolerate keep-alive / non-JSON frames
		}
		if chunk.Model != "" {
			respModel = chunk.Model
		}
		if chunk.Usage != nil {
			usage.PromptTokens = chunk.Usage.PromptTokens
			usage.CompletionTokens = chunk.Usage.CompletionTokens
			usage.TotalTokens = chunk.Usage.TotalTokens
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				contentB.WriteString(ch.Delta.Content)
				emit(ch.Delta.Content)
			}
			for _, tc := range ch.Delta.ToolCalls {
				acc := calls[tc.Index]
				if acc == nil {
					acc = &openAIToolCallAcc{}
					calls[tc.Index] = acc
					order = append(order, tc.Index)
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				acc.args.WriteString(tc.Function.Arguments)
			}
		}
		return false, nil
	})
	if err != nil {
		return ChatResult{}, err
	}

	out.Content = contentB.String()
	for _, idx := range order {
		acc := calls[idx]
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:   acc.id,
			Type: "function",
			Function: ToolCallFunction{
				Name:      acc.name,
				Arguments: acc.args.String(),
			},
		})
	}
	return ChatResult{Message: out, Usage: usage, Model: respModel}, nil
}
