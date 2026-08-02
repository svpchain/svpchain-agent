package a2a

import (
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// MessageText concatenates text parts from an A2A message.
func MessageText(msg *a2a.Message) string {
	if msg == nil || len(msg.Parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range msg.Parts {
		if part == nil {
			continue
		}
		if text := part.Text(); text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(text)
		}
	}
	return strings.TrimSpace(b.String())
}
