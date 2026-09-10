package settlement

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/svpchain/svpchain-agent/internal/agent/chainid"
)

const refundABIJSON = `[
  {"type":"function","name":"intentsOf","stateMutability":"view","inputs":[{"name":"payer","type":"address"}],"outputs":[{"name":"","type":"bytes32[]"}]},
  {"type":"function","name":"getIntent","stateMutability":"view","inputs":[{"name":"intentId","type":"bytes32"}],"outputs":[{"name":"payer","type":"address"},{"name":"deposited","type":"uint256"},{"name":"reserved","type":"uint256"},{"name":"settled","type":"uint256"},{"name":"refunded","type":"uint256"},{"name":"available","type":"uint256"}]},
  {"type":"function","name":"tasksOfIntent","stateMutability":"view","inputs":[{"name":"intentId","type":"bytes32"}],"outputs":[{"name":"","type":"bytes32[]"}]},
  {"type":"function","name":"getTask","stateMutability":"view","inputs":[{"name":"taskId","type":"bytes32"}],"outputs":[{"name":"intentId","type":"bytes32"},{"name":"owner","type":"address"},{"name":"amount","type":"uint256"},{"name":"txHash","type":"bytes32"},{"name":"agentIndex","type":"uint64"},{"name":"status","type":"uint8"},{"name":"createdAt","type":"uint64"},{"name":"settledAt","type":"uint64"}]},
  {"type":"function","name":"cancelTask","stateMutability":"nonpayable","inputs":[{"name":"taskId","type":"bytes32"}],"outputs":[]},
  {"type":"function","name":"refund","stateMutability":"nonpayable","inputs":[{"name":"intentId","type":"bytes32"}],"outputs":[]}
]`

// TaskStatus is AgentSettlement's on-chain task lifecycle value.
type TaskStatus uint8

const (
	TaskNone TaskStatus = iota
	TaskAssigned
	TaskBound
	TaskSuccess
	TaskFailed
	TaskCancelled
)

func (s TaskStatus) String() string {
	switch s {
	case TaskAssigned:
		return "assigned"
	case TaskBound:
		return "bound"
	case TaskSuccess:
		return "success"
	case TaskFailed:
		return "failed"
	case TaskCancelled:
		return "cancelled"
	default:
		return "none"
	}
}

// Active reports whether the task's reward is still reserved and therefore
// must not be made refundable from the desktop UI.
func (s TaskStatus) Active() bool { return s == TaskAssigned || s == TaskBound }

// Intent is the refund-relevant portion of an on-chain settlement intent.
type Intent struct {
	ID        string
	Payer     string
	Deposited string
	Reserved  string
	Settled   string
	Refunded  string
	Available string
}

// Task is the refund-relevant portion of an on-chain settlement task.
type OnChainTask struct {
	ID         string
	IntentID   string
	Owner      string
	Amount     string
	TxHash     string
	AgentIndex uint64
	Status     TaskStatus
}

// ListIntents returns all intents ever created by payer on this settlement
// contract. The contract, rather than local chat history, is the source of
// truth so failed tasks remain recoverable after their chat session ends.
func ListIntents(ctx context.Context, rpcURL, contract, payer string) ([]string, error) {
	address, err := requiredAddress("settlement contract", contract)
	if err != nil {
		return nil, err
	}
	if !common.IsHexAddress(strings.TrimSpace(payer)) {
		return nil, fmt.Errorf("payer %q is not a valid EVM address", payer)
	}
	values, err := callSettlement(ctx, rpcURL, address, "intentsOf", common.HexToAddress(payer))
	if err != nil {
		return nil, err
	}
	hashes, ok := values[0].([][32]byte)
	if !ok {
		return nil, fmt.Errorf("intentsOf returned %T", values[0])
	}
	out := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		out = append(out, common.Hash(hash).Hex())
	}
	return out, nil
}

func ReadIntent(ctx context.Context, rpcURL, contract, intentID string) (Intent, error) {
	address, err := requiredAddress("settlement contract", contract)
	if err != nil {
		return Intent{}, err
	}
	intent, err := settlementHash("intent_id", intentID)
	if err != nil {
		return Intent{}, err
	}
	values, err := callSettlement(ctx, rpcURL, address, "getIntent", intent)
	if err != nil {
		return Intent{}, err
	}
	if len(values) != 6 {
		return Intent{}, fmt.Errorf("getIntent returned %d values", len(values))
	}
	payer, ok := values[0].(common.Address)
	if !ok {
		return Intent{}, fmt.Errorf("getIntent payer has type %T", values[0])
	}
	amounts := make([]string, 5)
	for i := range amounts {
		amount, ok := values[i+1].(*big.Int)
		if !ok {
			return Intent{}, fmt.Errorf("getIntent amount %d has type %T", i, values[i+1])
		}
		amounts[i] = amount.String()
	}
	return Intent{ID: intent.Hex(), Payer: payer.Hex(), Deposited: amounts[0], Reserved: amounts[1], Settled: amounts[2], Refunded: amounts[3], Available: amounts[4]}, nil
}

func TasksOfIntent(ctx context.Context, rpcURL, contract, intentID string) ([]string, error) {
	address, err := requiredAddress("settlement contract", contract)
	if err != nil {
		return nil, err
	}
	intent, err := settlementHash("intent_id", intentID)
	if err != nil {
		return nil, err
	}
	values, err := callSettlement(ctx, rpcURL, address, "tasksOfIntent", intent)
	if err != nil {
		return nil, err
	}
	hashes, ok := values[0].([][32]byte)
	if !ok {
		return nil, fmt.Errorf("tasksOfIntent returned %T", values[0])
	}
	out := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		out = append(out, common.Hash(hash).Hex())
	}
	return out, nil
}

func ReadTask(ctx context.Context, rpcURL, contract, taskID string) (OnChainTask, error) {
	address, err := requiredAddress("settlement contract", contract)
	if err != nil {
		return OnChainTask{}, err
	}
	task, err := settlementHash("task_id", taskID)
	if err != nil {
		return OnChainTask{}, err
	}
	values, err := callSettlement(ctx, rpcURL, address, "getTask", task)
	if err != nil {
		return OnChainTask{}, err
	}
	if len(values) != 8 {
		return OnChainTask{}, fmt.Errorf("getTask returned %d values", len(values))
	}
	intent, ok := values[0].([32]byte)
	if !ok {
		return OnChainTask{}, fmt.Errorf("getTask intent ID has type %T", values[0])
	}
	owner, ok := values[1].(common.Address)
	if !ok {
		return OnChainTask{}, fmt.Errorf("getTask owner has type %T", values[1])
	}
	amount, ok := values[2].(*big.Int)
	if !ok {
		return OnChainTask{}, fmt.Errorf("getTask amount has type %T", values[2])
	}
	txHash, ok := values[3].([32]byte)
	if !ok {
		return OnChainTask{}, fmt.Errorf("getTask tx hash has type %T", values[3])
	}
	index, ok := values[4].(uint64)
	if !ok {
		return OnChainTask{}, fmt.Errorf("getTask agent index has type %T", values[4])
	}
	status, ok := values[5].(uint8)
	if !ok {
		return OnChainTask{}, fmt.Errorf("getTask status has type %T", values[5])
	}
	return OnChainTask{ID: task.Hex(), IntentID: common.Hash(intent).Hex(), Owner: owner.Hex(), Amount: amount.String(), TxHash: common.Hash(txHash).Hex(), AgentIndex: index, Status: TaskStatus(status)}, nil
}

// CancelTask releases an unbound task's reserved payment. AgentSettlement
// enforces that the signer is the original payer (or an orchestrator).
func CancelTask(ctx context.Context, cfg FundingConfig, taskID string) (string, error) {
	task, err := settlementHash("task_id", taskID)
	if err != nil {
		return "", err
	}
	return settlementAction(ctx, cfg, "cancelTask", task, "Cancel agent task", "Cancel the unbound agent task and release its reserved payment for refund.")
}

// Refund returns the currently available balance of an intent to its payer.
// AgentSettlement excludes active task reservations from the transferred sum.
func Refund(ctx context.Context, cfg FundingConfig, intentID string) (string, error) {
	intent, err := settlementHash("intent_id", intentID)
	if err != nil {
		return "", err
	}
	return settlementAction(ctx, cfg, "refund", intent, "Refund agent payment", "Return the available agent payment escrow to this wallet.")
}

func callSettlement(ctx context.Context, rpcURL string, contract common.Address, method string, args ...any) ([]any, error) {
	client, err := ethclient.DialContext(ctx, strings.TrimSpace(rpcURL))
	if err != nil {
		return nil, fmt.Errorf("connect settlement EVM RPC: %w", err)
	}
	defer client.Close()
	parsed, err := abi.JSON(strings.NewReader(refundABIJSON))
	if err != nil {
		return nil, err
	}
	data, err := parsed.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", method, err)
	}
	result, err := client.CallContract(ctx, ethereum.CallMsg{To: &contract, Data: data}, nil)
	if err != nil {
		return nil, fmt.Errorf("call settlement %s: %w", method, err)
	}
	values, err := parsed.Unpack(method, result)
	if err != nil {
		return nil, fmt.Errorf("decode settlement %s: %w", method, err)
	}
	return values, nil
}

func settlementAction(ctx context.Context, cfg FundingConfig, method string, arg common.Hash, title, description string) (string, error) {
	if cfg.PrivateKey == nil {
		return "", fmt.Errorf("settlement signing key is required")
	}
	if cfg.Confirm == nil {
		return "", fmt.Errorf("settlement action requires local confirmation")
	}
	chainID, ok := chainid.ParseEVM(strings.TrimSpace(cfg.ChainID))
	if !ok {
		return "", fmt.Errorf("cannot derive EVM chain ID from %q", cfg.ChainID)
	}
	contract, err := requiredAddress("settlement contract", cfg.SettlementContract)
	if err != nil {
		return "", err
	}
	client, err := ethclient.DialContext(ctx, strings.TrimSpace(cfg.RPCURL))
	if err != nil {
		return "", fmt.Errorf("connect settlement EVM RPC: %w", err)
	}
	defer client.Close()
	actualChainID, err := client.ChainID(ctx)
	if err != nil {
		return "", fmt.Errorf("read settlement EVM chain ID: %w", err)
	}
	if actualChainID.Cmp(new(big.Int).SetUint64(chainID)) != 0 {
		return "", fmt.Errorf("settlement EVM RPC chain ID %s does not match %s", actualChainID, cfg.ChainID)
	}
	parsed, err := abi.JSON(strings.NewReader(refundABIJSON))
	if err != nil {
		return "", err
	}
	data, err := parsed.Pack(method, arg)
	if err != nil {
		return "", fmt.Errorf("encode settlement %s: %w", method, err)
	}
	payer := common.BytesToAddress(cfg.PrivateKey.PubKey().Address())
	return signSendAndWait(ctx, client, cfg, payer, contract, data, title, description)
}

func settlementHash(label, value string) (common.Hash, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "0x")
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != common.HashLength {
		return common.Hash{}, fmt.Errorf("%s must be a 32-byte 0x hex value", label)
	}
	return common.BytesToHash(decoded), nil
}
