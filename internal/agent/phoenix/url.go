package phoenix

import (
	"net/url"
	"strings"
)

// DefaultURL is the local Phoenix OTLP traces endpoint.
const DefaultURL = "http://127.0.0.1:6006/v1/traces"

// ExportURL normalizes a user-supplied Phoenix OTLP URL. Empty or invalid
// input returns "" (export disabled). A host without a path gets /v1/traces.
func ExportURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return ""
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1/traces"
	}
	u.Fragment = ""
	return u.String()
}
