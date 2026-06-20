package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func toolResultText(res *mcp.CallToolResult) (string, error) {
	if res == nil {
		return "", fmt.Errorf("nil tool result")
	}
	if res.StructuredContent != nil {
		bz, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return "", err
		}
		return string(bz), nil
	}
	var parts []string
	for _, c := range res.Content {
		switch v := c.(type) {
		case *mcp.TextContent:
			if v.Text != "" {
				parts = append(parts, v.Text)
			}
		default:
			bz, err := json.Marshal(c)
			if err != nil {
				return "", err
			}
			parts = append(parts, string(bz))
		}
	}
	if len(parts) == 0 {
		return "{}", nil
	}
	return strings.Join(parts, "\n"), nil
}

func parseToolJSON[T any](res *mcp.CallToolResult) (T, error) {
	var out T
	text, err := toolResultText(res)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return out, fmt.Errorf("parse tool result: %w", err)
	}
	return out, nil
}
