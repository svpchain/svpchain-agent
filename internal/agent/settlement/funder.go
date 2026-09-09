package settlement

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/svpchain/svpchain-agent/internal/agent/chainid"
	"github.com/svpchain/svpchain-agent/internal/agent/hitl"
	"github.com/svpchain/svpchain-agent/internal/payload"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

const (
	erc20ABIJSON      = `[{"type":"function","name":"approve","stateMutability":"nonpayable","inputs":[{"name":"spender","type":"address"},{"name":"amount","type":"uint256"}],"outputs":[{"name":"","type":"bool"}]}]`
	settlementABIJSON = `[{"type":"function","name":"deposit","stateMutability":"nonpayable","inputs":[{"name":"intentId","type":"bytes32"},{"name":"amount","type":"uint256"}],"outputs":[]}]`
	erc20MetadataABI  = `[{"type":"function","name":"symbol","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"string"}]},{"type":"function","name":"decimals","stateMutability":"view","inputs":[],"outputs":[{"name":"","type":"uint8"}]}]`
)

// PaymentToken describes the ERC-20 selected by the validator's settlement
// deployment. Agent Market amounts are always expressed in this token's base
// units, never in the chain's native asvp denomination.
type PaymentToken struct {
	Address  string
	Symbol   string
	Decimals uint8
}

// ReadPaymentToken reads the human-facing ERC-20 metadata required to display
// a market quote. It is read-only and deliberately independent of funding.
func ReadPaymentToken(ctx context.Context, rpcURL, address string) (PaymentToken, error) {
	token, err := requiredAddress("payment token", address)
	if err != nil {
		return PaymentToken{}, err
	}
	client, err := ethclient.DialContext(ctx, strings.TrimSpace(rpcURL))
	if err != nil {
		return PaymentToken{}, fmt.Errorf("connect payment-token EVM RPC: %w", err)
	}
	defer client.Close()
	parsed, err := abi.JSON(strings.NewReader(erc20MetadataABI))
	if err != nil {
		return PaymentToken{}, err
	}
	symbolData, err := parsed.Pack("symbol")
	if err != nil {
		return PaymentToken{}, err
	}
	symbolResult, err := client.CallContract(ctx, ethereum.CallMsg{To: &token, Data: symbolData}, nil)
	if err != nil {
		return PaymentToken{}, fmt.Errorf("read payment-token symbol: %w", err)
	}
	symbolValues, err := parsed.Unpack("symbol", symbolResult)
	if err != nil || len(symbolValues) != 1 {
		return PaymentToken{}, fmt.Errorf("decode payment-token symbol")
	}
	symbol, ok := symbolValues[0].(string)
	if !ok || strings.TrimSpace(symbol) == "" {
		return PaymentToken{}, fmt.Errorf("payment-token symbol is empty")
	}
	decimalsData, err := parsed.Pack("decimals")
	if err != nil {
		return PaymentToken{}, err
	}
	decimalsResult, err := client.CallContract(ctx, ethereum.CallMsg{To: &token, Data: decimalsData}, nil)
	if err != nil {
		return PaymentToken{}, fmt.Errorf("read payment-token decimals: %w", err)
	}
	decimalsValues, err := parsed.Unpack("decimals", decimalsResult)
	if err != nil || len(decimalsValues) != 1 {
		return PaymentToken{}, fmt.Errorf("decode payment-token decimals")
	}
	decimals, ok := decimalsValues[0].(uint8)
	if !ok {
		return PaymentToken{}, fmt.Errorf("payment-token decimals have unexpected type %T", decimalsValues[0])
	}
	return PaymentToken{Address: token.Hex(), Symbol: strings.TrimSpace(symbol), Decimals: decimals}, nil
}

// FormatTokenAmount converts a base-unit integer to an exact, compact decimal
// string. It never rounds or exposes the token's precision to end users.
func FormatTokenAmount(amount string, decimals uint8) (string, error) {
	value, ok := new(big.Int).SetString(strings.TrimSpace(amount), 10)
	if !ok || value.Sign() < 0 {
		return "", fmt.Errorf("token amount must be a non-negative base-10 integer")
	}
	if decimals == 0 {
		return value.String(), nil
	}
	digits := value.String()
	precision := int(decimals)
	if len(digits) <= precision {
		digits = strings.Repeat("0", precision-len(digits)+1) + digits
	}
	whole, fraction := digits[:len(digits)-precision], digits[len(digits)-precision:]
	fraction = strings.TrimRight(fraction, "0")
	if fraction == "" {
		return whole, nil
	}
	return whole + "." + fraction, nil
}

// FundingConfig identifies the immutable network data needed to escrow one
// agent payment. Amount is in the payment token's smallest unit.
type FundingConfig struct {
	ChainID            string
	RPCURL             string
	SettlementContract string
	PaymentToken       string
	Amount             string
	PrivateKey         *ethsecp256k1.PrivKey
	Confirm            hitl.Func
}

// FundingResult is the funded intent and the two user-signed transaction IDs.
type FundingResult struct {
	IntentID      string
	TaskID        string
	ApproveTxHash string
	DepositTxHash string
}

// NewTaskIDs generates collision-resistant IDs compatible with
// AgentSettlement.computeIntentId/computeTaskId for payer.
func NewTaskIDs(payer common.Address) (intentID, taskID string, err error) {
	var intentSalt, taskSalt [32]byte
	if _, err := rand.Read(intentSalt[:]); err != nil {
		return "", "", fmt.Errorf("generate settlement intent salt: %w", err)
	}
	if _, err := rand.Read(taskSalt[:]); err != nil {
		return "", "", fmt.Errorf("generate settlement task salt: %w", err)
	}
	intent := crypto.Keccak256Hash(payer.Bytes(), intentSalt[:])
	task := crypto.Keccak256Hash(intent.Bytes(), taskSalt[:])
	return intent.Hex(), task.Hex(), nil
}

// EVMOwner converts the market's SVP owner address to its shared EVM address.
// A 0x address is accepted for compatibility with an EVM-native market record.
func EVMOwner(owner string) (common.Address, error) {
	owner = strings.TrimSpace(owner)
	if common.IsHexAddress(owner) {
		return common.HexToAddress(owner), nil
	}
	acc, err := sdk.AccAddressFromBech32(owner)
	if err != nil {
		return common.Address{}, fmt.Errorf("agent owner %q is neither a valid 0x nor SVP address", owner)
	}
	return common.BytesToAddress(acc.Bytes()), nil
}

// Fund approves and deposits exactly Amount into a new AgentSettlement intent.
// It waits for each transaction to succeed before returning, so callers may
// safely ask the validator to assign a task immediately afterwards.
func Fund(ctx context.Context, cfg FundingConfig) (FundingResult, error) {
	if cfg.PrivateKey == nil {
		return FundingResult{}, fmt.Errorf("settlement funding key is required")
	}
	if cfg.Confirm == nil {
		return FundingResult{}, fmt.Errorf("settlement funding requires local confirmation")
	}
	chainID, ok := chainid.ParseEVM(strings.TrimSpace(cfg.ChainID))
	if !ok {
		return FundingResult{}, fmt.Errorf("cannot derive EVM chain ID from %q", cfg.ChainID)
	}
	settlementAddr, err := requiredAddress("settlement contract", cfg.SettlementContract)
	if err != nil {
		return FundingResult{}, err
	}
	tokenAddr, err := requiredAddress("payment token", cfg.PaymentToken)
	if err != nil {
		return FundingResult{}, err
	}
	amount, ok := new(big.Int).SetString(strings.TrimSpace(cfg.Amount), 10)
	if !ok || amount.Sign() <= 0 {
		return FundingResult{}, fmt.Errorf("settlement amount must be a positive base-10 integer")
	}
	rpcURL := strings.TrimSpace(cfg.RPCURL)
	if rpcURL == "" {
		return FundingResult{}, fmt.Errorf("settlement EVM RPC URL is required")
	}

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return FundingResult{}, fmt.Errorf("connect settlement EVM RPC: %w", err)
	}
	defer client.Close()
	actualChainID, err := client.ChainID(ctx)
	if err != nil {
		return FundingResult{}, fmt.Errorf("read settlement EVM chain ID: %w", err)
	}
	if actualChainID.Cmp(new(big.Int).SetUint64(chainID)) != 0 {
		return FundingResult{}, fmt.Errorf("settlement EVM RPC chain ID %s does not match %s", actualChainID, cfg.ChainID)
	}

	payer := common.BytesToAddress(cfg.PrivateKey.PubKey().Address())
	intentID, taskID, err := NewTaskIDs(payer)
	if err != nil {
		return FundingResult{}, err
	}
	erc20ABI, err := abi.JSON(strings.NewReader(erc20ABIJSON))
	if err != nil {
		return FundingResult{}, err
	}
	approveData, err := erc20ABI.Pack("approve", settlementAddr, amount)
	if err != nil {
		return FundingResult{}, fmt.Errorf("encode settlement approval: %w", err)
	}
	approveHash, err := signSendAndWait(ctx, client, cfg, payer, tokenAddr, approveData,
		"Approve settlement payment token", "Allow the settlement contract to spend "+amount.String()+" payment-token base units.")
	if err != nil {
		return FundingResult{}, err
	}

	settlementABI, err := abi.JSON(strings.NewReader(settlementABIJSON))
	if err != nil {
		return FundingResult{}, err
	}
	intentHash := common.HexToHash(intentID)
	depositData, err := settlementABI.Pack("deposit", intentHash, amount)
	if err != nil {
		return FundingResult{}, fmt.Errorf("encode settlement deposit: %w", err)
	}
	depositHash, err := signSendAndWait(ctx, client, cfg, payer, settlementAddr, depositData,
		"Deposit agent payment", "Escrow "+amount.String()+" payment-token base units for this agent task.")
	if err != nil {
		return FundingResult{}, err
	}
	return FundingResult{IntentID: intentID, TaskID: taskID, ApproveTxHash: approveHash, DepositTxHash: depositHash}, nil
}

func requiredAddress(label, value string) (common.Address, error) {
	if !common.IsHexAddress(strings.TrimSpace(value)) {
		return common.Address{}, fmt.Errorf("%s %q is not a valid 0x address", label, value)
	}
	return common.HexToAddress(value), nil
}

func signSendAndWait(ctx context.Context, client *ethclient.Client, cfg FundingConfig, payer, to common.Address, data []byte, title, description string) (string, error) {
	nonce, err := client.PendingNonceAt(ctx, payer)
	if err != nil {
		return "", fmt.Errorf("read settlement nonce: %w", err)
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return "", fmt.Errorf("suggest settlement gas price: %w", err)
	}
	gas, err := client.EstimateGas(ctx, ethereum.CallMsg{From: payer, To: &to, GasPrice: gasPrice, Value: new(big.Int), Data: data})
	if err != nil {
		return "", fmt.Errorf("estimate %s gas: %w", strings.ToLower(title), err)
	}
	p := &payload.EvmTxPayload{
		Version:       payload.CurrentVersion,
		EVMChainID:    fmt.Sprint(chainIDFromCosmos(cfg.ChainID)),
		SignerAddress: payer.Hex(),
		TxType:        payload.EVMTxTypeLegacy,
		Nonce:         fmt.Sprint(nonce),
		To:            to.Hex(),
		Value:         "0",
		Gas:           fmt.Sprint(gas),
		GasPrice:      gasPrice.String(),
		Data:          hexutil.Encode(data),
		Summary:       payload.EvmSummary{ToolName: "agent_settlement", Description: description},
	}
	if !cfg.Confirm(ctx, hitl.SignRequest(hitl.KindSignEVM, map[string]any{"payload": p})) {
		return "", fmt.Errorf("%s was declined", strings.ToLower(title))
	}
	signed, err := signer.SignEvm(cfg.PrivateKey, p, cfg.ChainID)
	if err != nil {
		return "", fmt.Errorf("sign %s: %w", strings.ToLower(title), err)
	}
	raw, err := hexutil.Decode(signed.RawTxHex)
	if err != nil {
		return "", fmt.Errorf("decode signed settlement transaction: %w", err)
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(raw); err != nil {
		return "", fmt.Errorf("decode signed settlement transaction: %w", err)
	}
	if err := client.SendTransaction(ctx, &tx); err != nil {
		return "", fmt.Errorf("broadcast %s: %w", strings.ToLower(title), err)
	}
	receipt, err := bind.WaitMined(ctx, client, &tx)
	if err != nil {
		return tx.Hash().Hex(), fmt.Errorf("wait for %s %s: %w", strings.ToLower(title), tx.Hash().Hex(), err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return tx.Hash().Hex(), fmt.Errorf("%s transaction %s reverted", strings.ToLower(title), tx.Hash().Hex())
	}
	return tx.Hash().Hex(), nil
}

func chainIDFromCosmos(cosmosChainID string) uint64 {
	id, _ := chainid.ParseEVM(cosmosChainID)
	return id
}
