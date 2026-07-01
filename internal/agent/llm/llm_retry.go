package llm

import (
	"context"
	"errors"
	"strings"
	"time"
)

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

// withRetry retries do() on transient errors with exponential backoff, but stops
// retrying the moment any delta has been emitted (started bit) — re-running a stream
// after partial output would double tokens. The wrapped emit sets started on first call.
func (c *Client) withRetry(ctx context.Context, do func(emit func(string)) (chatRoundResult, error), onDelta func(string)) (chatRoundResult, error) {
	var lastErr error
	for attempt := 0; attempt <= llmMaxRetries; attempt++ {
		if ctx.Err() != nil {
			return chatRoundResult{}, ctx.Err()
		}
		started := false
		emit := func(s string) {
			started = true
			if onDelta != nil {
				onDelta(s)
			}
		}
		round, err := do(emit)
		if err == nil {
			return round, nil
		}
		lastErr = err
		// Do not retry once tokens have reached the caller, or for non-transient errors.
		if started || !isRetryable(err) || attempt == llmMaxRetries {
			return chatRoundResult{}, err
		}
		select {
		case <-ctx.Done():
			return chatRoundResult{}, ctx.Err()
		case <-time.After(llmRetryBaseDelay << attempt):
		}
	}
	return chatRoundResult{}, lastErr
}
