package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultLLMBaseURL  = "https://api.deepseek.com"
	defaultLLMModel    = "deepseek-v4-flash"
	anthropicBaseURL   = "https://api.anthropic.com"
	anthropicVersion   = "2023-06-01"
	anthropicMaxTokens = 40960
	llmMaxRetries      = 2
	llmRetryBaseDelay  = 1000 * time.Millisecond
	providerOpenAI     = "openai"
	providerAnthropic  = "anthropic"
)

// LLMConfig holds chat-completion API settings. Provider selects the wire format:
// "openai" covers every OpenAI-compatible service (deepseek, openai, openrouter,
// kimi, qwen, ollama, …); "anthropic" speaks the native /v1/messages format.
type LLMConfig struct {
	APIKey   string
	BaseURL  string
	Model    string
	Provider string
}

func (c LLMConfig) normalized() LLMConfig {
	out := c
	out.Provider = strings.ToLower(strings.TrimSpace(out.Provider))
	if out.Provider == "" {
		// Infer from the base URL host; default to the OpenAI-compatible family.
		if strings.Contains(strings.ToLower(out.BaseURL), "anthropic") {
			out.Provider = providerAnthropic
		} else {
			out.Provider = providerOpenAI
		}
	}
	if out.BaseURL == "" {
		if out.Provider == providerAnthropic {
			out.BaseURL = anthropicBaseURL
		} else {
			out.BaseURL = defaultLLMBaseURL
		}
	}
	out.BaseURL = strings.TrimRight(out.BaseURL, "/")
	if out.Model == "" {
		out.Model = defaultLLMModel
	}
	return out
}

type llmTool struct {
	Type     string      `json:"type"`
	Function llmFunction `json:"function"`
}

type llmFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type llmMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []llmToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

type llmToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// LLMClient calls a chat-completion API (OpenAI-compatible or Anthropic).
type LLMClient struct {
	cfg    LLMConfig
	client *http.Client
}

func NewLLMClient(cfg LLMConfig) *LLMClient {
	return &LLMClient{
		cfg:    cfg.normalized(),
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// Chat sends one round and returns the assistant message (with any tool calls).
// It streams under the hood: onDelta (if non-nil) receives assistant text increments
// as they arrive. Transient failures are retried — but only before the first delta is
// emitted, so a partially streamed answer is never duplicated.
func (c *LLMClient) Chat(ctx context.Context, messages []llmMessage, tools []llmTool, onDelta func(string)) (llmMessage, error) {
	if c.cfg.APIKey == "" {
		return llmMessage{}, fmt.Errorf("LLM API key is not configured")
	}
	if c.cfg.Provider == providerAnthropic {
		return c.withRetry(ctx, func(emit func(string)) (llmMessage, error) {
			return c.chatAnthropic(ctx, messages, tools, emit)
		}, onDelta)
	}
	return c.withRetry(ctx, func(emit func(string)) (llmMessage, error) {
		return c.chatOpenAI(ctx, messages, tools, emit)
	}, onDelta)
}

// withRetry retries do() on transient errors with exponential backoff, but stops
// retrying the moment any delta has been emitted (started bit) — re-running a stream
// after partial output would double tokens. The wrapped emit sets started on first call.
func (c *LLMClient) withRetry(ctx context.Context, do func(emit func(string)) (llmMessage, error), onDelta func(string)) (llmMessage, error) {
	var lastErr error
	for attempt := 0; attempt <= llmMaxRetries; attempt++ {
		if ctx.Err() != nil {
			return llmMessage{}, ctx.Err()
		}
		started := false
		emit := func(s string) {
			started = true
			if onDelta != nil {
				onDelta(s)
			}
		}
		msg, err := do(emit)
		if err == nil {
			return msg, nil
		}
		lastErr = err
		// Do not retry once tokens have reached the caller, or for non-transient errors.
		if started || !isRetryable(err) || attempt == llmMaxRetries {
			return llmMessage{}, err
		}
		select {
		case <-ctx.Done():
			return llmMessage{}, ctx.Err()
		case <-time.After(llmRetryBaseDelay << attempt):
		}
	}
	return llmMessage{}, lastErr
}

// retryableError marks an HTTP-status failure as worth retrying (429 / 5xx).
type retryableError struct{ msg string }

func (e *retryableError) Error() string { return e.msg }

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	// Explicitly-classified transient HTTP statuses (429 / 5xx).
	var re *retryableError
	if errors.As(err, &re) {
		return true
	}
	// Non-retryable HTTP responses (4xx other than 429) are returned as plain fmt
	// errors with this prefix; everything else (dial/reset/EOF) is transport-level
	// and worth a retry.
	return !strings.HasPrefix(err.Error(), "LLM HTTP 4")
}

// ---- OpenAI-compatible (streaming) ----

type openAIChatRequest struct {
	Model    string       `json:"model"`
	Messages []llmMessage `json:"messages"`
	Tools    []llmTool    `json:"tools,omitempty"`
	Stream   bool         `json:"stream"`
}

func (c *LLMClient) chatOpenAI(ctx context.Context, messages []llmMessage, tools []llmTool, emit func(string)) (llmMessage, error) {
	body, err := json.Marshal(openAIChatRequest{Model: c.cfg.Model, Messages: messages, Tools: tools, Stream: true})
	if err != nil {
		return llmMessage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return llmMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return llmMessage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return llmMessage{}, httpError(resp)
	}

	out := llmMessage{Role: "assistant"}
	var contentB strings.Builder
	// tool_calls arrive sharded by index across frames; accumulate per index.
	type tcAcc struct {
		id, name string
		args     strings.Builder
	}
	calls := map[int]*tcAcc{}
	var order []int

	err = scanSSE(resp.Body, func(data string) (bool, error) {
		if data == "[DONE]" {
			return true, nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
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
					acc = &tcAcc{}
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
		return llmMessage{}, err
	}

	out.Content = contentB.String()
	for _, idx := range order {
		acc := calls[idx]
		var call llmToolCall
		call.ID = acc.id
		call.Type = "function"
		call.Function.Name = acc.name
		call.Function.Arguments = acc.args.String()
		out.ToolCalls = append(out.ToolCalls, call)
	}
	return out, nil
}

// ---- Anthropic native (/v1/messages, streaming) ----

func (c *LLMClient) chatAnthropic(ctx context.Context, messages []llmMessage, tools []llmTool, emit func(string)) (llmMessage, error) {
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
		return llmMessage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return llmMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	resp, err := c.client.Do(req)
	if err != nil {
		return llmMessage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return llmMessage{}, httpError(resp)
	}

	out := llmMessage{Role: "assistant"}
	var contentB strings.Builder
	// content blocks arrive by index; a tool_use block accumulates partial_json.
	type blk struct {
		kind, id, name string
		json           strings.Builder
	}
	blocks := map[int]*blk{}
	var order []int

	err = scanSSE(resp.Body, func(data string) (bool, error) {
		var ev struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return false, nil
		}
		switch ev.Type {
		case "content_block_start":
			b := &blk{kind: ev.ContentBlock.Type, id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
			blocks[ev.Index] = b
			order = append(order, ev.Index)
		case "content_block_delta":
			b := blocks[ev.Index]
			if b == nil {
				b = &blk{}
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
		return llmMessage{}, err
	}

	out.Content = contentB.String()
	for _, idx := range order {
		b := blocks[idx]
		if b.kind != "tool_use" {
			continue
		}
		var call llmToolCall
		call.ID = b.id
		call.Type = "function"
		call.Function.Name = b.name
		args := b.json.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		call.Function.Arguments = args
		out.ToolCalls = append(out.ToolCalls, call)
	}
	return out, nil
}

// toAnthropicMessages converts OpenAI-shaped messages into Anthropic's (system, messages)
// pair: system text is hoisted out; tool results become a user tool_result block; an
// assistant message with tool calls becomes tool_use blocks.
func toAnthropicMessages(messages []llmMessage) (system string, out []map[string]any) {
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

func toAnthropicTools(tools []llmTool) []map[string]any {
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

// ---- shared helpers ----

// scanSSE reads a text/event-stream body and calls onData for each `data:` payload.
// onData returns stop=true to end early (e.g. on [DONE] / message_stop).
func scanSSE(r io.Reader, onData func(data string) (stop bool, err error)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		stop, err := onData(data)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	return sc.Err()
}

// httpError reads an error response body and classifies 429/5xx as retryable.
func httpError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	msg := fmt.Sprintf("LLM HTTP %d: %s", resp.StatusCode, truncate(string(body), 500))
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return &retryableError{msg: msg}
	}
	return fmt.Errorf("%s", msg)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
