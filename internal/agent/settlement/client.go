package settlement

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/svpchain/svpchain-agent/internal/agent/netretry"
)

// validatorTimeout bounds a validator call. CreateTask broadcasts assignTask
// and waits for its receipt before replying, so this has to cover block
// inclusion rather than a round trip: observed inclusion here reaches ~51s.
// The previous 15s cap sat inside that range, reporting failure for
// assignments that then succeeded and aborting the run after the deposit had
// already moved user funds.
const validatorTimeout = 120 * time.Second

// Client calls the validator's private task-assignment API. The callback token
// remains local to the process; callers only provide public settlement fields.
type Client struct {
	validatorURL  string
	callbackToken string
	httpClient    *http.Client
}

// Task is the funded on-chain intent that must be assigned before execution.
// All values are passed to AgentSettlement unchanged after validation by the
// validator service.
type Task struct {
	IntentID   string `json:"intent_id"`
	TaskID     string `json:"task_id"`
	Amount     string `json:"amount"`
	Owner      string `json:"owner"`
	AgentIndex string `json:"agent_index"`
}

// AgentIndexFromID returns the index encoded in a canonical SVP agent DID.
// Historical did:svp:<owner> records remain addressable as index zero; new
// records use did:svp:<owner>:<positive decimal index>.
func AgentIndexFromID(agentID string) (string, error) {
	agentID = strings.TrimSpace(agentID)
	const prefix = "did:svp:"
	if !strings.HasPrefix(agentID, prefix) {
		return "", fmt.Errorf("agent_id %q must start with %q", agentID, prefix)
	}
	parts := strings.Split(strings.TrimPrefix(agentID, prefix), ":")
	if len(parts) == 1 && strings.TrimSpace(parts[0]) != "" {
		return "0", nil
	}
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || parts[1] == "" {
		return "", fmt.Errorf("agent_id %q has an invalid DID suffix", agentID)
	}
	if strings.Trim(parts[1], "0123456789") != "" {
		return "", fmt.Errorf("agent_id %q has a non-decimal index", agentID)
	}
	index, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil || index == 0 || strconv.FormatUint(index, 10) != parts[1] {
		return "", fmt.Errorf("agent_id %q must have a canonical positive index", agentID)
	}
	return parts[1], nil
}

// Assignment is the validator response after assignTask is mined.
type Assignment struct {
	TaskID           string `json:"task_id"`
	State            string `json:"state"`
	AssignmentTxHash string `json:"assignment_tx_hash"`
}

// NetworkConfig is the network-level settlement deployment returned by the
// validator. Agent price and owner remain Agent Market data.
type NetworkConfig struct {
	SettlementContract string `json:"settlement_contract"`
	PaymentToken       string `json:"payment_token"`
}

// NewClient constructs a validator client for one trusted local deployment.
func NewClient(validatorURL, callbackToken string) (*Client, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(validatorURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("settlement validator URL must be an absolute HTTP URL")
	}
	return &Client{
		validatorURL:  strings.TrimRight(parsed.String(), "/"),
		callbackToken: strings.TrimSpace(callbackToken),
		httpClient:    &http.Client{Timeout: validatorTimeout},
	}, nil
}

// NetworkConfig reads the validator's configured settlement contract/token.
// This is a read-only endpoint and needs no shared callback secret.
func (c *Client) NetworkConfig(ctx context.Context) (NetworkConfig, error) {
	if c == nil {
		return NetworkConfig{}, fmt.Errorf("settlement validator client is nil")
	}
	resp, err := c.do(ctx, http.MethodGet, "/v1/settlement/config", nil)
	if err != nil {
		return NetworkConfig{}, fmt.Errorf("query settlement network config: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return NetworkConfig{}, fmt.Errorf("query settlement network config returned %s", resp.Status)
	}
	var config NetworkConfig
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return NetworkConfig{}, fmt.Errorf("decode settlement network config: %w", err)
	}
	if strings.TrimSpace(config.SettlementContract) == "" || strings.TrimSpace(config.PaymentToken) == "" {
		return NetworkConfig{}, fmt.Errorf("settlement network config is incomplete")
	}
	return config, nil
}

// CreateTask asks the validator to assign a funded task. It is idempotent for
// an already assigned task with the same on-chain parameters.
func (c *Client) CreateTask(ctx context.Context, task Task) (Assignment, error) {
	if c == nil {
		return Assignment{}, fmt.Errorf("settlement validator client is nil")
	}
	body, err := json.Marshal(struct {
		IntentID   string `json:"intent_id"`
		TaskID     string `json:"task_id"`
		Amount     string `json:"amount"`
		Owner      string `json:"owner"`
		AgentIndex string `json:"agent_index"`
	}{
		IntentID:   strings.TrimSpace(task.IntentID),
		TaskID:     strings.TrimSpace(task.TaskID),
		Amount:     strings.TrimSpace(task.Amount),
		Owner:      strings.TrimSpace(task.Owner),
		AgentIndex: strings.TrimSpace(task.AgentIndex),
	})
	if err != nil {
		return Assignment{}, err
	}
	// The validator creates a task idempotently for this immutable intent/task
	// pair, so retrying a lost HTTP response cannot fund or assign a second task.
	resp, err := c.do(ctx, http.MethodPost, "/internal/v1/tasks", body)
	if err != nil {
		return Assignment{}, fmt.Errorf("create settlement task: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		// The validator explains refusals in the body; the status alone is not
		// enough to tell a reverted assignTask from a rejected argument.
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return Assignment{}, fmt.Errorf("create settlement task returned %s: %s", resp.Status, bytes.TrimSpace(detail))
	}
	var assignment Assignment
	if err := json.NewDecoder(resp.Body).Decode(&assignment); err != nil {
		return Assignment{}, fmt.Errorf("decode settlement task response: %w", err)
	}
	return assignment, nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var response *http.Response
	err := netretry.Do(ctx, func() error {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.validatorURL+path, reader)
		if err != nil {
			return err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.callbackToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.callbackToken)
		}
		response, err = c.httpClient.Do(req)
		return err
	})
	return response, err
}
