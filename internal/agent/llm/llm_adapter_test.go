package llm

import (
	"context"
	"fmt"
	"testing"
)

type fakeModel struct {
	calls int
}

func (f *fakeModel) Chat(_ context.Context, _ []Message, _ []Tool, emit func(string)) (ChatResult, error) {
	f.calls++
	if emit != nil {
		emit("hi")
	}
	return ChatResult{
		Message: Message{Role: "assistant", Content: "hi"},
		Usage:   Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		Model:   "fake-model",
	}, nil
}

func TestNewClientWithModel_doesNotHitHTTP(t *testing.T) {
	fake := &fakeModel{}
	c := NewClientWithModel(Config{APIKey: "k", Model: "ignored"}, fake)
	var deltas string
	res, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "ping"}}, nil, func(s string) {
		deltas += s
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls)
	}
	if deltas != "hi" || res.Message.Content != "hi" {
		t.Errorf("content=%q deltas=%q", res.Message.Content, deltas)
	}
	if res.Model != "fake-model" {
		t.Errorf("model = %q", res.Model)
	}
	if res.LatencyMs < 0 {
		t.Errorf("latency = %d", res.LatencyMs)
	}
}

func TestNewClientWithModel_retriesBeforeFirstDelta(t *testing.T) {
	fake := &failOnceModel{}
	c := NewClientWithModel(Config{APIKey: "k", Model: "m"}, fake)
	res, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "ping"}}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if fake.n != 2 {
		t.Fatalf("calls = %d, want 2 (one retryable fail + success)", fake.n)
	}
	if res.Message.Content != "ok" {
		t.Errorf("content = %q", res.Message.Content)
	}
}

type failOnceModel struct{ n int }

func (f *failOnceModel) Chat(_ context.Context, _ []Message, _ []Tool, _ func(string)) (ChatResult, error) {
	f.n++
	if f.n == 1 {
		return ChatResult{}, fmt.Errorf("temporary")
	}
	return ChatResult{Message: Message{Role: "assistant", Content: "ok"}}, nil
}

func TestNewModel_selectsProvider(t *testing.T) {
	o := newModel(Config{Provider: providerOpenAI}, nil)
	if _, ok := o.(*openaiModel); !ok {
		t.Fatalf("openai provider: %T", o)
	}
	a := newModel(Config{Provider: providerAnthropic}, nil)
	if _, ok := a.(*anthropicModel); !ok {
		t.Fatalf("anthropic provider: %T", a)
	}
}
