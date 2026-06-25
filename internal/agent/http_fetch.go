package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPFetch performs an HTTP request and returns status + body (truncated if huge).
func HTTPFetch(method, url string, headers map[string]string, body string) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bz, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", err
	}
	out := map[string]any{
		"status":      resp.StatusCode,
		"status_text": resp.Status,
		"headers":     headerMap(resp.Header),
		"body":        string(bz),
	}
	if resp.StatusCode == http.StatusPaymentRequired {
		if pay := firstHeader(resp.Header, "Payment-Required", "PAYMENT-REQUIRED", "X-PAYMENT-REQUIREMENTS"); pay != "" {
			out["payment_required"] = pay
		}
		if www := resp.Header.Get("WWW-Authenticate"); www != "" {
			out["www_authenticate"] = www
		}
		if submit := firstHeader(resp.Header, "X-Payment-Submit-Header"); submit != "" {
			out["payment_submit_header"] = submit
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func headerMap(h http.Header) map[string]string {
	m := make(map[string]string, len(h))
	for k, vs := range h {
		if len(vs) > 0 {
			m[k] = vs[0]
		}
	}
	return m
}

func firstHeader(h http.Header, names ...string) string {
	for _, name := range names {
		for k, vs := range h {
			if strings.EqualFold(k, name) && len(vs) > 0 && vs[0] != "" {
				return vs[0]
			}
		}
	}
	return ""
}

// BuildXPaymentHeader encodes an x402 payment payload for the X-PAYMENT header.
func BuildXPaymentHeader(payload map[string]any) (string, error) {
	bz, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bz), nil
}

// HTTPFetchFromArgs parses tool arguments for http_fetch.
func HTTPFetchFromArgs(args map[string]any) (string, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url is required")
	}
	method, _ := args["method"].(string)
	headers := map[string]string{}
	if raw, ok := args["headers"].(map[string]any); ok {
		for k, v := range raw {
			if s, ok := v.(string); ok {
				headers[k] = s
			}
		}
	}
	body, _ := args["body"].(string)
	return HTTPFetch(method, url, headers, body)
}

func isHttpTool(name string) bool {
	switch name {
	case "http_fetch":
		return true
	default:
		return false
	}
}
