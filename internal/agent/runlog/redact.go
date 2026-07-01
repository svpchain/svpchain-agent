package runlog

import (
	"regexp"
	"strings"
)

var (
	reHexKey    = regexp.MustCompile(`(?i)(0x)?[0-9a-f]{64}`)
	reAPIKeyish = regexp.MustCompile(`(?i)(api[_-]?key|authorization|bearer)\s*[:=]\s*\S+`)
)

const maxFieldLen = 2000

// Redact shortens and removes likely secrets from trace fields.
func Redact(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = reAPIKeyish.ReplaceAllString(s, "$1=[REDACTED]")
	s = reHexKey.ReplaceAllStringFunc(s, func(m string) string {
		if len(m) >= 64 {
			return "[REDACTED_KEY]"
		}
		return m
	})
	if len(s) > maxFieldLen {
		return s[:maxFieldLen] + "…"
	}
	return s
}
