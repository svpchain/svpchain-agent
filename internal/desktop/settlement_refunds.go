package desktop

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/errgroup"

	"github.com/svpchain/svpchain-agent/internal/agent/history"
	agentsettlement "github.com/svpchain/svpchain-agent/internal/agent/settlement"
	"github.com/svpchain/svpchain-agent/internal/keystore"
	"github.com/svpchain/svpchain-agent/internal/manage"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

// SettlementRefund is one local wallet's on-chain payment intent. It is read
// from AgentSettlement rather than conversation history, so a validator-marked
// failed task remains visible after its chat session has been cleared.
type SettlementRefund struct {
	ChainID         string `json:"chain_id"`
	Payer           string `json:"payer"`
	IntentID        string `json:"intent_id"`
	DisplayIntentID string `json:"display_intent_id"`
	TaskID          string `json:"task_id,omitempty"`
	TaskStatus      string `json:"task_status"`
	Amount          string `json:"amount,omitempty"`
	Available       string `json:"available"`
	Token           string `json:"token"`
	Refundable      bool   `json:"refundable"`
	Cancellable     bool   `json:"cancellable"`
	ValidatorState  string `json:"validator_state,omitempty"`
	ValidatorError  string `json:"validator_error,omitempty"`
}

// SettlementRefundPage is a newest-first page of settlement intents. A single
// intent can expand to more than one task row.
type SettlementRefundPage struct {
	Rows     []SettlementRefund `json:"rows"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Total    int                `json:"total"`
}

const (
	defaultSettlementPageSize = 10
	maxSettlementPageSize     = 25
	settlementReadConcurrency = 4
)

// SettlementRefunds lists intents created by the locally stored keys and their
// current contract state. A refund is deliberately withheld while any task in
// the intent is Assigned or Bound, even though the contract would only return
// its unreserved balance.
func (a *App) SettlementRefunds(page, pageSize int) (SettlementRefundPage, error) {
	ctx, cancel := a.settlementContext(30 * time.Second)
	defer cancel()
	page, pageSize = normalizeSettlementPage(page, pageSize)
	network, err := a.settlementNetwork(ctx)
	if err != nil {
		return SettlementRefundPage{}, localized(err)
	}
	token, err := agentsettlement.ReadPaymentToken(ctx, settlementRPCURL(), network.PaymentToken)
	if err != nil {
		return SettlementRefundPage{}, localized(err)
	}
	validator, err := agentsettlement.NewClient(resolveSettlementValidatorURL(a.AgentGetSettings()), "")
	if err != nil {
		return SettlementRefundPage{}, localized(err)
	}
	entries, err := manage.List()
	if err != nil {
		return SettlementRefundPage{}, localized(err)
	}
	reader, err := agentsettlement.NewReader(ctx, settlementRPCURL(), network.SettlementContract)
	if err != nil {
		return SettlementRefundPage{}, localized(err)
	}
	defer reader.Close()

	seen := make(map[string]struct{}, len(entries))
	type intentRef struct{ chainID, intentID string }
	var refs []intentRef
	for _, entry := range entries {
		if strings.TrimSpace(entry.ChainID) == "" || !common.IsHexAddress(entry.EVMAddr) {
			continue
		}
		key := entry.ChainID + "|" + strings.ToLower(entry.EVMAddr)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		intentIDs, err := reader.ListIntents(ctx, entry.EVMAddr)
		if err != nil {
			return SettlementRefundPage{}, localized(fmt.Errorf("read settlement intents for %s: %w", entry.EVMAddr, err))
		}
		// AgentSettlement appends intent IDs in creation order. Iterate from
		// the tail so the first page contains the newest payments.
		for i := len(intentIDs) - 1; i >= 0; i-- {
			refs = append(refs, intentRef{chainID: entry.ChainID, intentID: intentIDs[i]})
		}
	}
	total := len(refs)
	start := (page - 1) * pageSize
	if start >= total {
		return SettlementRefundPage{Rows: []SettlementRefund{}, Page: page, PageSize: pageSize, Total: total}, nil
	}
	end := min(start+pageSize, total)
	pageRefs := refs[start:end]
	rowSets := make([][]SettlementRefund, len(pageRefs))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(settlementReadConcurrency)
	for i, ref := range pageRefs {
		i, ref := i, ref
		group.Go(func() error {
			rows, err := readSettlementIntent(groupCtx, reader, validator, ref.chainID, ref.intentID, token)
			if err != nil {
				return err
			}
			rowSets[i] = rows
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return SettlementRefundPage{}, localized(err)
	}
	rows := make([]SettlementRefund, 0, len(pageRefs))
	for _, intentRows := range rowSets {
		rows = append(rows, intentRows...)
	}
	return SettlementRefundPage{Rows: rows, Page: page, PageSize: pageSize, Total: total}, nil
}

func readSettlementIntent(ctx context.Context, reader *agentsettlement.Reader, validator *agentsettlement.Client, chainID, intentID string, token agentsettlement.PaymentToken) ([]SettlementRefund, error) {
	intent, err := reader.ReadIntent(ctx, intentID)
	if err != nil {
		return nil, err
	}
	taskIDs, err := reader.TasksOfIntent(ctx, intentID)
	if err != nil {
		return nil, err
	}
	tasks := make([]agentsettlement.OnChainTask, 0, len(taskIDs))
	// Task IDs are also append-ordered in the contract, so show the newest task
	// first within each intent.
	for i := len(taskIDs) - 1; i >= 0; i-- {
		task, err := reader.ReadTask(ctx, taskIDs[i])
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	rows := settlementRefundRows(chainID, intent, tasks, token)
	for i := range rows {
		if rows[i].TaskID == "" || rows[i].TaskStatus == "unassigned" {
			continue
		}
		execution, found, err := validator.Execution(ctx, rows[i].TaskID)
		if err != nil || !found {
			continue
		}
		rows[i].ValidatorState = execution.State
		rows[i].ValidatorError = execution.LastError
		rows[i].TaskStatus = settlementDisplayStatus(execution.State, execution.LastError, rows[i].TaskStatus)
	}
	return rows, nil
}

func normalizeSettlementPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultSettlementPageSize
	}
	if pageSize > maxSettlementPageSize {
		pageSize = maxSettlementPageSize
	}
	return page, pageSize
}

// settlementDisplayStatus uses the validator's persisted lifecycle when a
// callback exists. Refund eligibility remains based on the contract state.
func settlementDisplayStatus(state, lastError, fallback string) string {
	switch strings.TrimSpace(state) {
	case "received":
		if strings.TrimSpace(lastError) != "" {
			return "retrying"
		}
		return "submitted"
	case "bound", "pending":
		return "validating"
	case "succeeded":
		return "success"
	case "failed":
		return "failed"
	default:
		return fallback
	}
}

// SettlementCancel cancels an unbound task through the payer's local signer.
// It does not refund automatically: the resulting released balance is shown in
// the refund list and needs its own explicit confirmation.
func (a *App) SettlementCancel(chainID, taskID string) (string, error) {
	ctx, cancel := a.settlementContext(90 * time.Second)
	defer cancel()
	cfg, payer, err := a.settlementActionConfig(ctx, chainID)
	if err != nil {
		return "", localized(err)
	}
	task, err := agentsettlement.ReadTask(ctx, cfg.RPCURL, cfg.SettlementContract, taskID)
	if err != nil {
		return "", localized(err)
	}
	if task.Status != agentsettlement.TaskAssigned {
		return "", localized(fmt.Errorf("task %s is %s and cannot be cancelled", taskID, task.Status))
	}
	if err := a.requireSettlementPayer(ctx, cfg, payer, task.IntentID); err != nil {
		return "", localized(err)
	}
	txHash, err := agentsettlement.CancelTask(ctx, cfg, taskID)
	if err != nil {
		return txHash, localized(err)
	}
	if err := history.Shared().ClearActiveSettlementTask(taskID); err != nil {
		return txHash, localized(fmt.Errorf("clear cancelled settlement from local sessions: %w", err))
	}
	return txHash, nil
}

// SettlementRefund returns an intent's released balance to its payer. The
// desktop rejects an intent with any in-flight task, so an executing task can
// never be refunded through this UI.
func (a *App) SettlementRefund(chainID, intentID string) (string, error) {
	ctx, cancel := a.settlementContext(90 * time.Second)
	defer cancel()
	cfg, payer, err := a.settlementActionConfig(ctx, chainID)
	if err != nil {
		return "", localized(err)
	}
	if err := a.requireSettlementPayer(ctx, cfg, payer, intentID); err != nil {
		return "", localized(err)
	}
	taskIDs, err := agentsettlement.TasksOfIntent(ctx, cfg.RPCURL, cfg.SettlementContract, intentID)
	if err != nil {
		return "", localized(err)
	}
	for _, taskID := range taskIDs {
		task, err := agentsettlement.ReadTask(ctx, cfg.RPCURL, cfg.SettlementContract, taskID)
		if err != nil {
			return "", localized(err)
		}
		if task.Status.Active() {
			return "", localized(fmt.Errorf("task %s is still %s and cannot be refunded", taskID, task.Status))
		}
	}
	intent, err := agentsettlement.ReadIntent(ctx, cfg.RPCURL, cfg.SettlementContract, intentID)
	if err != nil {
		return "", localized(err)
	}
	if !positiveAmount(intent.Available) {
		return "", localized(fmt.Errorf("settlement intent %s has no refundable balance", intentID))
	}
	txHash, err := agentsettlement.Refund(ctx, cfg, intentID)
	if err != nil {
		return txHash, localized(err)
	}
	if err := history.Shared().ClearActiveSettlementIntent(intentID); err != nil {
		return txHash, localized(fmt.Errorf("clear refunded settlement from local sessions: %w", err))
	}
	return txHash, nil
}

func settlementRefundRows(chainID string, intent agentsettlement.Intent, tasks []agentsettlement.OnChainTask, token agentsettlement.PaymentToken) []SettlementRefund {
	available := displaySettlementAmount(intent.Available, token)
	refundable := positiveAmount(intent.Available)
	for _, task := range tasks {
		if task.Status.Active() {
			refundable = false
			break
		}
	}
	base := SettlementRefund{
		ChainID: chainID, Payer: intent.Payer, IntentID: intent.ID, DisplayIntentID: intent.ID, Available: available, Token: token.Symbol, Refundable: refundable,
	}
	if len(tasks) == 0 {
		// Reuse the stable task_id wire field as the action key for an intent
		// that was funded but never assigned. There is no on-chain task yet.
		base.TaskID = intent.ID
		base.TaskStatus = "unassigned"
		return []SettlementRefund{base}
	}
	rows := make([]SettlementRefund, 0, len(tasks))
	for _, task := range tasks {
		row := base
		// getTask returns the canonical bytes32 intent id. Prefer it for task
		// rows so the UI always displays the task's on-chain association.
		row.IntentID = task.IntentID
		row.DisplayIntentID = task.IntentID
		row.TaskID = task.ID
		row.TaskStatus = task.Status.String()
		row.Amount = displaySettlementAmount(task.Amount, token)
		row.Cancellable = task.Status == agentsettlement.TaskAssigned
		rows = append(rows, row)
	}
	return rows
}

// SettlementRefundEntry refunds the released balance associated with a table
// entry. Task entries resolve their intent from getTask; unassigned entries use
// their ID as the intent because no task exists on chain yet.
func (a *App) SettlementRefundEntry(chainID, id string, unassigned bool) (string, error) {
	if unassigned {
		return a.SettlementRefund(chainID, id)
	}
	ctx, cancel := a.settlementContext(30 * time.Second)
	defer cancel()
	cfg, _, err := a.settlementActionConfig(ctx, chainID)
	if err != nil {
		return "", localized(err)
	}
	task, err := agentsettlement.ReadTask(ctx, cfg.RPCURL, cfg.SettlementContract, id)
	if err != nil {
		return "", localized(err)
	}
	if task.Status == agentsettlement.TaskNone {
		return "", localized(fmt.Errorf("settlement task %s does not exist", id))
	}
	return a.SettlementRefund(chainID, task.IntentID)
}

func (a *App) settlementActionConfig(ctx context.Context, chainID string) (agentsettlement.FundingConfig, common.Address, error) {
	chainID = strings.TrimSpace(chainID)
	if chainID == "" {
		return agentsettlement.FundingConfig{}, common.Address{}, fmt.Errorf("settlement chain ID is required")
	}
	network, err := a.settlementNetwork(ctx)
	if err != nil {
		return agentsettlement.FundingConfig{}, common.Address{}, err
	}
	ring, err := keystore.Open()
	if err != nil {
		return agentsettlement.FundingConfig{}, common.Address{}, err
	}
	hexKey, _, err := manage.SelectKey(ring, chainID, os.Getenv("SIGNER_KEY_HEX"))
	if err != nil {
		return agentsettlement.FundingConfig{}, common.Address{}, err
	}
	priv, err := signer.ParsePrivKey(hexKey)
	if err != nil {
		return agentsettlement.FundingConfig{}, common.Address{}, fmt.Errorf("parse key: %w", err)
	}
	payer := common.BytesToAddress(priv.PubKey().Address())
	return agentsettlement.FundingConfig{
		ChainID: chainID, RPCURL: settlementRPCURL(), SettlementContract: network.SettlementContract,
		PaymentToken: network.PaymentToken, PrivateKey: priv, Confirm: a.confirmHook,
	}, payer, nil
}

func (a *App) requireSettlementPayer(ctx context.Context, cfg agentsettlement.FundingConfig, payer common.Address, intentID string) error {
	intent, err := agentsettlement.ReadIntent(ctx, cfg.RPCURL, cfg.SettlementContract, intentID)
	if err != nil {
		return err
	}
	if !strings.EqualFold(intent.Payer, payer.Hex()) {
		return fmt.Errorf("this wallet is not the payer for settlement intent %s", intentID)
	}
	return nil
}

func (a *App) settlementNetwork(ctx context.Context) (agentsettlement.NetworkConfig, error) {
	client, err := agentsettlement.NewClient(resolveSettlementValidatorURL(a.AgentGetSettings()), "")
	if err != nil {
		return agentsettlement.NetworkConfig{}, err
	}
	return client.NetworkConfig(ctx)
}

func (a *App) settlementContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, timeout)
}

func displaySettlementAmount(value string, token agentsettlement.PaymentToken) string {
	formatted, err := agentsettlement.FormatTokenAmount(value, token.Decimals)
	if err != nil {
		formatted = value
	}
	if token.Symbol == "" {
		return formatted
	}
	return formatted + " " + token.Symbol
}

func positiveAmount(value string) bool {
	amount, ok := new(big.Int).SetString(strings.TrimSpace(value), 10)
	return ok && amount.Sign() > 0
}
