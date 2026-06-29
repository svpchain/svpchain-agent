package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type openAIChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
	Stream   bool      `json:"stream"`
}

// openAIStreamChunk is one SSE `data:` frame from /v1/chat/completions (stream=true).
type openAIStreamChunk struct {
	Choices []openAIStreamChoice `json:"choices"`
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

func (c *Client) chatOpenAI(ctx context.Context, messages []Message, tools []Tool, emit func(string)) (Message, error) {
	body, err := json.Marshal(openAIChatRequest{Model: c.cfg.Model, Messages: messages, Tools: tools, Stream: true})
	if err != nil {
		return Message{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Message{}, httpError(resp)
	}

	out := Message{Role: "assistant"}
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
		return Message{}, err
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
	return out, nil
}
