package llm

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
)

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
