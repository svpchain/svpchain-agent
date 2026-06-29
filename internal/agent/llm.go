package agent

import (
	"context"
	"fmt"
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
// emitted, so a partially streamed answer is never duplicated. The provider-specific
// wire handling lives in chatOpenAI / chatAnthropic.
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
