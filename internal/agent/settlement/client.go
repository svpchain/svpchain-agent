package settlement

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
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
	IntentID string `json:"intent_id"`
	TaskID   string `json:"task_id"`
	Amount   string `json:"amount"`
	Owner    string `json:"owner"`
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.validatorURL+"/v1/settlement/config", nil)
	if err != nil {
		return NetworkConfig{}, err
	}
	resp, err := c.httpClient.Do(req)
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
		IntentID string `json:"intent_id"`
		TaskID   string `json:"task_id"`
		Amount   string `json:"amount"`
		Owner    string `json:"owner"`
	}{
		IntentID: strings.TrimSpace(task.IntentID),
		TaskID:   strings.TrimSpace(task.TaskID),
		Amount:   strings.TrimSpace(task.Amount),
		Owner:    strings.TrimSpace(task.Owner),
	})
	if err != nil {
		return Assignment{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.validatorURL+"/internal/v1/tasks", bytes.NewReader(body))
	if err != nil {
		return Assignment{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.callbackToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.callbackToken)
	}
	resp, err := c.httpClient.Do(req)
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
