package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultLLMBaseURL = "https://api.deepseek.com"
	defaultLLMModel   = "deepseek-v4-flash"
)

// LLMConfig holds OpenAI-compatible API settings.
type LLMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

func (c LLMConfig) normalized() LLMConfig {
	out := c
	if out.BaseURL == "" {
		out.BaseURL = defaultLLMBaseURL
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

type llmChatRequest struct {
	Model    string       `json:"model"`
	Messages []llmMessage `json:"messages"`
	Tools    []llmTool    `json:"tools,omitempty"`
}

type llmChatResponse struct {
	Choices []struct {
		Message llmMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// LLMClient calls an OpenAI-compatible chat completions API.
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

func (c *LLMClient) Chat(ctx context.Context, messages []llmMessage, tools []llmTool) (llmMessage, error) {
	if c.cfg.APIKey == "" {
		return llmMessage{}, fmt.Errorf("LLM API key is not configured")
	}
	reqBody := llmChatRequest{
		Model:    c.cfg.Model,
		Messages: messages,
		Tools:    tools,
	}
	bz, err := json.Marshal(reqBody)
	if err != nil {
		return llmMessage{}, err
	}
	url := c.cfg.BaseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bz))
	if err != nil {
		return llmMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return llmMessage{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return llmMessage{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return llmMessage{}, fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, truncate(string(body), 500))
	}
	var out llmChatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return llmMessage{}, err
	}
	if out.Error != nil {
		return llmMessage{}, fmt.Errorf("LLM error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return llmMessage{}, fmt.Errorf("LLM returned no choices")
	}
	return out.Choices[0].Message, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
