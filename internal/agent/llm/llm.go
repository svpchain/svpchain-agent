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

// Model is the LLM transport (the ChatModel adapter). An implementation speaks
// one provider's wire format and streams tokens. It MUST NOT execute tools —
// the runner owns dispatch (whitelist → write-path → HITL).
type Model interface {
	Chat(ctx context.Context, messages []Message, tools []Tool, emit func(string)) (ChatResult, error)
}

// Client wraps a Model with retries and latency accounting.
type Client struct {
	cfg   Config
	model Model
}

// NewClient picks the OpenAI-compatible or Anthropic adapter from cfg.Provider.
func NewClient(cfg Config) *Client {
	cfg = cfg.normalized()
	httpClient := &http.Client{Timeout: 120 * time.Second}
	return NewClientWithModel(cfg, newModel(cfg, httpClient))
}

// NewClientWithModel uses a caller-supplied transport. Tests inject fakes;
// a future Eino/Genkit wrapper would plug in here without touching runner.go.
func NewClientWithModel(cfg Config, model Model) *Client {
	cfg = cfg.normalized()
	if model == nil {
		model = newModel(cfg, nil)
	}
	return &Client{cfg: cfg, model: model}
}

func newModel(cfg Config, httpClient *http.Client) Model {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}
	if cfg.Provider == providerAnthropic {
		return &anthropicModel{cfg: cfg, client: httpClient}
	}
	return &openaiModel{cfg: cfg, client: httpClient}
}

// Chat sends one round and returns the assistant message (with any tool calls),
// per-round latency, and token usage when the provider reports it in the stream.
// It streams under the hood: onDelta (if non-nil) receives assistant text increments
// as they arrive. Transient failures are retried — but only before the first delta is
// emitted, so a partially streamed answer is never duplicated.
func (c *Client) Chat(ctx context.Context, messages []Message, tools []Tool, onDelta func(string)) (ChatResult, error) {
	if c.cfg.APIKey == "" {
		return ChatResult{}, fmt.Errorf("LLM API key is not configured")
	}
	start := time.Now()
	round, err := c.withRetry(ctx, func(emit func(string)) (ChatResult, error) {
		return c.model.Chat(ctx, messages, tools, emit)
	}, onDelta)
	if err != nil {
		return ChatResult{}, err
	}
	if round.Model == "" {
		round.Model = c.cfg.Model
	}
	round.LatencyMs = time.Since(start).Milliseconds()
	return round, nil
}
