package llm

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

// Config holds chat-completion API settings. Provider selects the wire format:
// "openai" covers every OpenAI-compatible service (deepseek, openai, openrouter,
// kimi, qwen, ollama, …); "anthropic" speaks the native /v1/messages format.
type Config struct {
	APIKey   string
	BaseURL  string
	Model    string
	Provider string
}

func (c Config) normalized() Config {
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

// Client calls a chat-completion API (OpenAI-compatible or Anthropic).
type Client struct {
	cfg    Config
	client *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{
		cfg:    cfg.normalized(),
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// Chat sends one round and returns the assistant message (with any tool calls),
// per-round latency, and token usage when the provider reports it in the stream.
// It streams under the hood: onDelta (if non-nil) receives assistant text increments
// as they arrive. Transient failures are retried — but only before the first delta is
// emitted, so a partially streamed answer is never duplicated. The provider-specific
// wire handling lives in chatOpenAI / chatAnthropic.
func (c *Client) Chat(ctx context.Context, messages []Message, tools []Tool, onDelta func(string)) (ChatResult, error) {
	if c.cfg.APIKey == "" {
		return ChatResult{}, fmt.Errorf("LLM API key is not configured")
	}
	start := time.Now()
	var round chatRoundResult
	var err error
	if c.cfg.Provider == providerAnthropic {
		round, err = c.withRetry(ctx, func(emit func(string)) (chatRoundResult, error) {
			return c.chatAnthropic(ctx, messages, tools, emit)
		}, onDelta)
	} else {
		round, err = c.withRetry(ctx, func(emit func(string)) (chatRoundResult, error) {
			return c.chatOpenAI(ctx, messages, tools, emit)
		}, onDelta)
	}
	if err != nil {
		return ChatResult{}, err
	}
	model := round.model
	if model == "" {
		model = c.cfg.Model
	}
	return ChatResult{
		Message:   round.msg,
		Usage:     round.usage,
		Model:     model,
		LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}
