package runlog

import (
	"encoding/json"
	"regexp"
	"strings"
)

var reTxHash = regexp.MustCompile(`(?i)(?:0x)?[0-9a-f]{64}`)

// ExtractTxHashes collects transaction hashes from tool JSON/text results.
func ExtractTxHashes(toolName, result string) []string {
	result = strings.TrimSpace(result)
	if result == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(h string) {
		h = normalizeTxHash(h)
		if h == "" {
			return
		}
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}

	if strings.HasPrefix(result, "{") || strings.HasPrefix(result, "[") {
		var v any
		if json.Unmarshal([]byte(result), &v) == nil {
			collectTxHashesJSON(v, add)
		}
	}
	if strings.Contains(strings.ToLower(toolName), "broadcast") {
		for _, m := range reTxHash.FindAllString(result, -1) {
			add(m)
		}
	}
	return out
}

func normalizeTxHash(h string) string {
	h = strings.TrimSpace(strings.ToLower(h))
	h = strings.TrimPrefix(h, "0x")
	if len(h) != 64 {
		return ""
	}
	return "0x" + h
}

func collectTxHashesJSON(v any, add func(string)) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			lk := strings.ToLower(k)
			if strings.Contains(lk, "tx_hash") || strings.Contains(lk, "transaction_hash") ||
				lk == "hash" || lk == "txhash" {
				if s, ok := val.(string); ok {
					add(s)
				}
			}
			collectTxHashesJSON(val, add)
		}
	case []any:
		for _, item := range x {
			collectTxHashesJSON(item, add)
		}
	}
}
