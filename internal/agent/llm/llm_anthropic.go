package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// anthropicStreamEvent is one SSE `data:` frame from /v1/messages (stream=true).
type anthropicStreamEvent struct {
	Type         string                `json:"type"`
	Index        int                   `json:"index"`
	ContentBlock anthropicContentBlock `json:"content_block"`
	Delta        anthropicStreamDelta  `json:"delta"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type anthropicStreamDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	PartialJSON string `json:"partial_json"`
}

// anthropicBlockAcc accumulates one content block (text or tool_use) across frames.
type anthropicBlockAcc struct {
	kind string
	id   string
	name string
	json strings.Builder
}

func (c *Client) chatAnthropic(ctx context.Context, messages []Message, tools []Tool, emit func(string)) (Message, error) {
	system, msgs := toAnthropicMessages(messages)
	reqBody := map[string]any{
		"model":      c.cfg.Model,
		"max_tokens": anthropicMaxTokens,
		"messages":   msgs,
		"stream":     true,
	}
	if system != "" {
		reqBody["system"] = system
	}
	if atools := toAnthropicTools(tools); len(atools) > 0 {
		reqBody["tools"] = atools
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return Message{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
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
	// content blocks arrive by index; a tool_use block accumulates partial_json.
	blocks := map[int]*anthropicBlockAcc{}
	var order []int

	err = scanSSE(resp.Body, func(data string) (bool, error) {
		var ev anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return false, nil
		}
		switch ev.Type {
		case "content_block_start":
			blocks[ev.Index] = &anthropicBlockAcc{kind: ev.ContentBlock.Type, id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
			order = append(order, ev.Index)
		case "content_block_delta":
			b := blocks[ev.Index]
			if b == nil {
				b = &anthropicBlockAcc{}
				blocks[ev.Index] = b
				order = append(order, ev.Index)
			}
			switch ev.Delta.Type {
			case "text_delta":
				contentB.WriteString(ev.Delta.Text)
				emit(ev.Delta.Text)
			case "input_json_delta":
				b.json.WriteString(ev.Delta.PartialJSON)
			}
		case "message_stop":
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return Message{}, err
	}

	out.Content = contentB.String()
	for _, idx := range order {
		b := blocks[idx]
		if b.kind != "tool_use" {
			continue
		}
		args := b.json.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:   b.id,
			Type: "function",
			Function: ToolCallFunction{
				Name:      b.name,
				Arguments: args,
			},
		})
	}
	return out, nil
}

// toAnthropicMessages converts OpenAI-shaped messages into Anthropic's (system, messages)
// pair: system text is hoisted out; tool results become a user tool_result block; an
// assistant message with tool calls becomes tool_use blocks.
func toAnthropicMessages(messages []Message) (system string, out []map[string]any) {
	var sys []string
	for _, m := range messages {
		switch m.Role {
		case "system":
			sys = append(sys, m.Content)
		case "tool":
			out = append(out, map[string]any{
				"role": "user",
				"content": []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     m.Content,
				}},
			})
		case "assistant":
			content := []map[string]any{}
			if m.Content != "" {
				content = append(content, map[string]any{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				var input any
				if tc.Function.Arguments != "" {
					_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
				}
				if input == nil {
					input = map[string]any{}
				}
				content = append(content, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": input,
				})
			}
			out = append(out, map[string]any{"role": "assistant", "content": content})
		default: // user
			out = append(out, map[string]any{"role": "user", "content": m.Content})
		}
	}
	return strings.Join(sys, "\n\n"), out
}

func toAnthropicTools(tools []Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		schema := t.Function.Parameters
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"name":         t.Function.Name,
			"description":  t.Function.Description,
			"input_schema": schema,
		})
	}
	return out
}
