package chainrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/svpchain/svpchain-agent/internal/agent/runlog"
)

const maxBodyBytes = 4 << 20

// Lookup returns a run-log querier for one CometBFT RPC base URL.
// An empty URL yields a nil querier (tx checks are recorded as skipped).
func Lookup(baseURL string) runlog.TxQuerier {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return nil
	}
	client := &http.Client{Timeout: 15 * time.Second}
	return func(ctx context.Context, hash string) (runlog.ChainTx, error) {
		return lookupTx(ctx, client, base, hash)
	}
}

type rpcEnvelope struct {
	Error  *rpcError `json:"error"`
	Result *rpcTx    `json:"result"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type rpcTx struct {
	Hash     string          `json:"hash"`
	Height   json.RawMessage `json:"height"`
	TxResult rpcTxResult     `json:"tx_result"`
}

type rpcTxResult struct {
	Code   uint32     `json:"code"`
	Log    string     `json:"log"`
	Events []rpcEvent `json:"events"`
}

type rpcEvent struct {
	Type       string         `json:"type"`
	Attributes []rpcAttribute `json:"attributes"`
}

type rpcAttribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func lookupTx(ctx context.Context, client *http.Client, base, hash string) (runlog.ChainTx, error) {
	qhash := QueryHash(hash)
	if qhash == "" {
		return runlog.ChainTx{}, fmt.Errorf("chain rpc: empty tx hash")
	}
	full := base + "/tx?hash=" + url.QueryEscape(qhash)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return runlog.ChainTx{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return runlog.ChainTx{}, fmt.Errorf("chain rpc: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return runlog.ChainTx{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return runlog.ChainTx{}, fmt.Errorf("chain rpc: %s: %s", resp.Status, clipBody(body))
	}
	var env rpcEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return runlog.ChainTx{}, fmt.Errorf("chain rpc: decode: %w", err)
	}
	if env.Error != nil {
		return runlog.ChainTx{}, fmt.Errorf("chain rpc: %s", env.Error.Error())
	}
	if env.Result == nil {
		return runlog.ChainTx{}, fmt.Errorf("chain rpc: tx not found")
	}
	return chainTxFromRPC(*env.Result), nil
}

func (e rpcError) Error() string {
	msg := strings.TrimSpace(e.Message)
	if d := strings.TrimSpace(rpcErrorData(e.Data)); d != "" {
		if msg == "" {
			return d
		}
		return msg + ": " + d
	}
	if msg == "" {
		return "unknown error"
	}
	return msg
}

func rpcErrorData(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return strings.TrimSpace(string(raw))
}

func chainTxFromRPC(tx rpcTx) runlog.ChainTx {
	events := make([]runlog.ChainEvent, 0, len(tx.TxResult.Events))
	for _, e := range tx.TxResult.Events {
		attrs := make(map[string]string, len(e.Attributes))
		for _, a := range e.Attributes {
			if k := strings.TrimSpace(a.Key); k != "" {
				attrs[k] = a.Value
			}
		}
		events = append(events, runlog.ChainEvent{Type: e.Type, Attrs: attrs})
	}
	return runlog.ChainTx{
		Code:   tx.TxResult.Code,
		Height: parseHeight(tx.Height),
		RawLog: tx.TxResult.Log,
		Events: events,
	}
}

func parseHeight(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return strings.Trim(strings.TrimSpace(string(raw)), `"`)
}

func clipBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
