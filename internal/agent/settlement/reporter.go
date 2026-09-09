// Package settlement reports one explicitly configured agent behavior to the
// internal validator service. It deliberately does not infer a settlement task
// from a chat run or an A2A task ID.
package settlement

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/svpchain/svpchain-agent/internal/agent/netretry"
	"github.com/svpchain/svpchain-agent/internal/agent/runlog"
)

const (
	SourceEVM                = "evm"
	SourceCosmosDelegatedEVM = "cosmos_delegated_evm"
)

// Config identifies an already assigned AgentSettlement task. TaskID is the
// settlement contract bytes32 task ID, not an A2A task identifier.
type Config struct {
	TaskID        string
	Owner         string
	ValidatorURL  string
	CallbackToken string
	Source        string
}

// Reporter records the final successful execution broadcast and reports its
// hash to the validator after the enclosing run has finished. A settlement task
// may cover a workflow whose prerequisite transactions all need to be mined;
// the final broadcast is the workflow's completion signal.
type Reporter struct {
	cfg             *Config
	client          *http.Client
	mu              sync.Mutex
	finalHash       string
	approvalClients map[string]struct{}
}

// Callback returns the task and final transaction for the completed behavior.
// It is intended for post-callback UI status only; false means the run did not
// broadcast an execution transaction.
func (r *Reporter) Callback() (taskID, txHash string, ok bool) {
	if r == nil {
		return "", "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cfg == nil || r.finalHash == "" {
		return "", "", false
	}
	return r.cfg.TaskID, r.finalHash, true
}

func New(cfg Config) (*Reporter, error) {
	reporter := NewDeferred()
	if err := reporter.Configure(cfg); err != nil {
		return nil, err
	}
	return reporter, nil
}

// NewDeferred creates an inactive reporter. Configure activates it once the
// user-funded task has been assigned; broadcasts observed before then are
// intentionally ignored.
func NewDeferred() *Reporter {
	return &Reporter{client: &http.Client{Timeout: 10 * time.Second}}
}

// Configure validates and activates a reporter for one user-requested behavior
// represented by one settlement task. That behavior may require multiple
// sequential on-chain transactions.
func (r *Reporter) Configure(cfg Config) error {
	if r == nil {
		return fmt.Errorf("settlement reporter is nil")
	}
	cfg.TaskID = normalizeHash(cfg.TaskID)
	if cfg.TaskID == "" {
		return fmt.Errorf("settlement task_id must be a 32-byte 0x hex value")
	}
	parsed, err := url.ParseRequestURI(strings.TrimSpace(cfg.ValidatorURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("settlement validator URL must be an absolute HTTP URL")
	}
	cfg.ValidatorURL = strings.TrimRight(parsed.String(), "/")
	cfg.Owner = strings.TrimSpace(cfg.Owner)
	cfg.Source = strings.TrimSpace(cfg.Source)
	if cfg.Source == "" {
		cfg.Source = SourceEVM
	}
	if cfg.Source != SourceEVM && cfg.Source != SourceCosmosDelegatedEVM {
		return fmt.Errorf("unsupported settlement source %q", cfg.Source)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cfg != nil {
		return fmt.Errorf("settlement reporter is already configured")
	}
	r.cfg = &cfg
	return nil
}

// RecordTool implements agent.ToolObserver.
func (r *Reporter) RecordTool(name, args string) func(ok bool, result, _ string) {
	if r == nil {
		return func(bool, string, string) {}
	}
	return func(ok bool, result, _ string) {
		// A settlement task is paid by approve + deposit before the agent is
		// contacted. Their hashes (and validator assignment hashes) can appear
		// in the a2a_connect_agent result after Configure has run. They are not
		// the agent's execution and must never be reported as one. Only a tool
		// that actually broadcasts the requested work may produce tx_hash.
		if !ok {
			return
		}
		if isApprovalBuild(name) {
			r.recordApprovalClient(extractPayloadClientID(result))
			return
		}
		if !isExecutionBroadcast(name) {
			return
		}
		r.mu.Lock()
		configured := r.cfg != nil
		r.mu.Unlock()
		if !configured {
			return
		}
		if r.isApprovalClient(extractClientID(args)) {
			return
		}
		for _, hash := range runlog.ExtractTxHashes(name, result) {
			r.setFinal(hash)
		}
	}
}

func isApprovalBuild(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "build_token_approval" || name == "build_erc20_approve"
}

func extractPayloadClientID(result string) string {
	var value struct {
		Payload struct {
			ClientID string `json:"client_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(result), &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value.Payload.ClientID)
}

func extractClientID(args string) string {
	var value struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal([]byte(args), &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value.ClientID)
}

func (r *Reporter) recordApprovalClient(clientID string) {
	if clientID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cfg == nil {
		return
	}
	if r.approvalClients == nil {
		r.approvalClients = make(map[string]struct{})
	}
	r.approvalClients[clientID] = struct{}{}
}

func (r *Reporter) isApprovalClient(clientID string) bool {
	if clientID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.approvalClients[clientID]
	return ok
}

func isExecutionBroadcast(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.Contains(name, "broadcast") || name == "execute_delegated_evm"
}

// Report posts the final successful execution transaction hash. No collected
// hash is a no-op because nothing broadcast.
func (r *Reporter) Report(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.cfg == nil {
		r.mu.Unlock()
		return nil
	}
	cfg := *r.cfg
	finalHash := r.finalHash
	r.mu.Unlock()
	if finalHash == "" {
		return nil
	}
	// agent-validator deliberately accepts only this immutable execution
	// identity. Owner belongs to task assignment and source is client-side
	// telemetry; sending either fails its strict JSON decoder.
	body, err := json.Marshal(struct {
		TaskID string `json:"task_id"`
		TxHash string `json:"tx_hash"`
	}{TaskID: cfg.TaskID, TxHash: finalHash})
	if err != nil {
		return err
	}
	var resp *http.Response
	err = netretry.Do(ctx, func() error {
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ValidatorURL+"/internal/v1/executions", bytes.NewReader(body))
		if requestErr != nil {
			return requestErr
		}
		req.Header.Set("Content-Type", "application/json")
		if token := strings.TrimSpace(cfg.CallbackToken); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, requestErr = r.client.Do(req)
		return requestErr
	})
	if err != nil {
		return fmt.Errorf("report settlement execution: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("report settlement execution returned %s", resp.Status)
	}
	return nil
}

func (r *Reporter) setFinal(hash string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalHash = hash
}

func normalizeHash(value string) string {
	value = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(value)), "0x")
	if len(value) != 64 {
		return ""
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return ""
		}
	}
	return "0x" + value
}
