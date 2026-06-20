package signer

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"

	"github.com/svpchain/svpchain-mcp/internal/payload"
)

// PrimaryTypeTransferWithAuthorization is the EIP-3009 typed-data primary type used by x402 "exact".
const PrimaryTypeTransferWithAuthorization = "TransferWithAuthorization"

// PrimaryTypePermitWitnessTransferFrom is the Permit2 typed-data primary type used by x402 permit2 fallback.
const PrimaryTypePermitWitnessTransferFrom = "PermitWitnessTransferFrom"

// Permit2DomainName is the EIP-712 domain name for the canonical Permit2 contract.
const Permit2DomainName = "Permit2"

// CanonicalPermit2Address is the CREATE2 Permit2 contract address on supported EVM chains.
const CanonicalPermit2Address = "0x000000000022D473030F116dDEE9F6B43aC78BA3"

var transferWithAuthorizationFields = []payload.EIP712Type{
	{Name: "from", Type: "address"},
	{Name: "to", Type: "address"},
	{Name: "value", Type: "uint256"},
	{Name: "validAfter", Type: "uint256"},
	{Name: "validBefore", Type: "uint256"},
	{Name: "nonce", Type: "bytes32"},
}

var permitWitnessTransferFromFields = []payload.EIP712Type{
	{Name: "permitted", Type: "TokenPermissions"},
	{Name: "spender", Type: "address"},
	{Name: "nonce", Type: "uint256"},
	{Name: "deadline", Type: "uint256"},
	{Name: "witness", Type: "Witness"},
}

var tokenPermissionsFields = []payload.EIP712Type{
	{Name: "token", Type: "address"},
	{Name: "amount", Type: "uint256"},
}

var permit2WitnessFields = []payload.EIP712Type{
	{Name: "to", Type: "address"},
	{Name: "validAfter", Type: "uint256"},
}

// SignTypedData signs EIP-712 typed data for allowed x402 shapes:
// EIP-3009 TransferWithAuthorization and Permit2 PermitWitnessTransferFrom.
//
// Guardrails (mirror sign_evm_transaction):
//   - evmChainID must be non-zero (EVM signing configured)
//   - primaryType must be on the allowlist
//   - domain.chainId must equal evmChainID
//   - TransferWithAuthorization: message.from must equal the key-derived 0x address
//   - PermitWitnessTransferFrom: domain must target canonical Permit2; recovered signer must match key
func SignTypedData(priv *ethsecp256k1.PrivKey, td *payload.EIP712TypedData, evmChainID uint64) (*payload.SignedTypedData, error) {
	if evmChainID == 0 {
		return nil, fmt.Errorf("EVM signing is not configured: start the signer with --evm-chain-id (or an evmos-style --chain-id)")
	}
	if td == nil {
		return nil, fmt.Errorf("typed_data is required")
	}

	domainChainID, err := parseDomainChainID(td.Domain.ChainId)
	if err != nil {
		return nil, err
	}
	if domainChainID != evmChainID {
		return nil, fmt.Errorf(
			"domain.chainId %d does not match signer evm chain id %d",
			domainChainID, evmChainID,
		)
	}

	keyAddr := common.HexToAddress(DeriveEvmAddress(priv))

	switch td.PrimaryType {
	case PrimaryTypeTransferWithAuthorization:
		if err := validateTransferWithAuthorization(td); err != nil {
			return nil, err
		}
		from, err := messageAddress(td.Message, "from")
		if err != nil {
			return nil, err
		}
		if from != keyAddr {
			return nil, fmt.Errorf("key-derived signer address %s does not match message.from %s",
				keyAddr.Hex(), from.Hex())
		}
	case PrimaryTypePermitWitnessTransferFrom:
		if err := validatePermitWitnessTransferFrom(td); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf(
			"primaryType %q is not allowed — supported: %q (EIP-3009), %q (Permit2)",
			td.PrimaryType, PrimaryTypeTransferWithAuthorization, PrimaryTypePermitWitnessTransferFrom,
		)
	}

	apiTD, err := toAPITypedData(td)
	if err != nil {
		return nil, err
	}
	digest, _, err := apitypes.TypedDataAndHash(apiTD)
	if err != nil {
		return nil, fmt.Errorf("hash typed data: %w", err)
	}

	ecdsaKey, err := ethcrypto.ToECDSA(priv.Key)
	if err != nil {
		return nil, fmt.Errorf("convert key to ecdsa: %w", err)
	}
	sig, err := ethcrypto.Sign(digest, ecdsaKey)
	if err != nil {
		return nil, fmt.Errorf("sign typed data: %w", err)
	}
	// crypto.Sign returns v as 0/1; Ethereum wallets use 27/28.
	sig[64] += 27

	if td.PrimaryType == PrimaryTypePermitWitnessTransferFrom {
		recovered, err := recoverSigner(digest, sig)
		if err != nil {
			return nil, err
		}
		if recovered != keyAddr {
			return nil, fmt.Errorf("signature recovery mismatch: expected %s", keyAddr.Hex())
		}
	}

	return &payload.SignedTypedData{
		Signature: hexutil.Encode(sig),
		Signer:    keyAddr.Hex(),
	}, nil
}

func validateTransferWithAuthorization(td *payload.EIP712TypedData) error {
	if err := validateTypeFields(td.Types, PrimaryTypeTransferWithAuthorization, transferWithAuthorizationFields); err != nil {
		return err
	}
	if strings.TrimSpace(td.Domain.VerifyingContract) == "" {
		return fmt.Errorf("domain.verifyingContract is required")
	}
	if !common.IsHexAddress(td.Domain.VerifyingContract) {
		return fmt.Errorf("domain.verifyingContract %q is not a valid 0x address", td.Domain.VerifyingContract)
	}
	return nil
}

func validatePermitWitnessTransferFrom(td *payload.EIP712TypedData) error {
	if err := validateTypeFields(td.Types, PrimaryTypePermitWitnessTransferFrom, permitWitnessTransferFromFields); err != nil {
		return err
	}
	if err := validateTypeFields(td.Types, "TokenPermissions", tokenPermissionsFields); err != nil {
		return err
	}
	if err := validateTypeFields(td.Types, "Witness", permit2WitnessFields); err != nil {
		return err
	}
	if strings.TrimSpace(td.Domain.Name) != Permit2DomainName {
		return fmt.Errorf("domain.name must be %q for Permit2, got %q", Permit2DomainName, td.Domain.Name)
	}
	if !sameAddress(td.Domain.VerifyingContract, CanonicalPermit2Address) {
		return fmt.Errorf("domain.verifyingContract must be canonical Permit2 %s, got %q",
			CanonicalPermit2Address, td.Domain.VerifyingContract)
	}
	if _, err := messageAddress(td.Message, "spender"); err != nil {
		return err
	}
	if err := validateNestedAddress(td.Message, "witness", "to"); err != nil {
		return err
	}
	return nil
}

func validateTypeFields(types map[string][]payload.EIP712Type, typeName string, want []payload.EIP712Type) error {
	fields, ok := types[typeName]
	if !ok {
		return fmt.Errorf("types.%s is required", typeName)
	}
	if len(fields) != len(want) {
		return fmt.Errorf("types.%s must have %d fields", typeName, len(want))
	}
	for i, w := range want {
		got := fields[i]
		if got.Name != w.Name || got.Type != w.Type {
			return fmt.Errorf("types.%s field %d: want {name:%q type:%q}, got {name:%q type:%q}",
				typeName, i, w.Name, w.Type, got.Name, got.Type)
		}
	}
	return nil
}

func validateNestedAddress(msg map[string]interface{}, objField, addrField string) error {
	if msg == nil {
		return fmt.Errorf("message.%s is required", objField)
	}
	raw, ok := msg[objField]
	if !ok || raw == nil {
		return fmt.Errorf("message.%s is required", objField)
	}
	nested, ok := raw.(map[string]interface{})
	if !ok {
		return fmt.Errorf("message.%s must be an object", objField)
	}
	_, err := messageAddress(nested, addrField)
	return err
}

func sameAddress(a, b string) bool {
	if !common.IsHexAddress(a) || !common.IsHexAddress(b) {
		return false
	}
	return common.HexToAddress(a) == common.HexToAddress(b)
}

func recoverSigner(digest, sig []byte) (common.Address, error) {
	if len(sig) != 65 {
		return common.Address{}, fmt.Errorf("invalid signature length %d", len(sig))
	}
	recoverSig := make([]byte, 65)
	copy(recoverSig, sig)
	if recoverSig[64] >= 27 {
		recoverSig[64] -= 27
	}
	pub, err := ethcrypto.SigToPub(digest, recoverSig)
	if err != nil {
		return common.Address{}, fmt.Errorf("recover signer: %w", err)
	}
	return ethcrypto.PubkeyToAddress(*pub), nil
}

func parseDomainChainID(v interface{}) (uint64, error) {
	if v == nil {
		return 0, fmt.Errorf("domain.chainId is required")
	}
	switch x := v.(type) {
	case float64:
		if x < 0 || x != float64(uint64(x)) {
			return 0, fmt.Errorf("domain.chainId %v is not a valid uint64", v)
		}
		return uint64(x), nil
	case string:
		x = strings.TrimSpace(x)
		if x == "" {
			return 0, fmt.Errorf("domain.chainId is required")
		}
		if strings.HasPrefix(x, "0x") || strings.HasPrefix(x, "0X") {
			n, err := hexutil.DecodeUint64(x)
			if err != nil {
				return 0, fmt.Errorf("parse domain.chainId %q: %w", x, err)
			}
			return n, nil
		}
		n, err := strconv.ParseUint(x, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse domain.chainId %q: %w", x, err)
		}
		return n, nil
	case json.Number:
		n, err := x.Int64()
		if err != nil || n < 0 {
			return 0, fmt.Errorf("parse domain.chainId %q: %w", x, err)
		}
		return uint64(n), nil
	default:
		return 0, fmt.Errorf("domain.chainId has unsupported type %T", v)
	}
}

func messageAddress(msg map[string]interface{}, field string) (common.Address, error) {
	if msg == nil {
		return common.Address{}, fmt.Errorf("message.%s is required", field)
	}
	raw, ok := msg[field]
	if !ok || raw == nil {
		return common.Address{}, fmt.Errorf("message.%s is required", field)
	}
	s, ok := raw.(string)
	if !ok {
		return common.Address{}, fmt.Errorf("message.%s must be a string address", field)
	}
	if !common.IsHexAddress(s) {
		return common.Address{}, fmt.Errorf("message.%s %q is not a valid 0x address", field, s)
	}
	return common.HexToAddress(s), nil
}

func toAPITypedData(td *payload.EIP712TypedData) (apitypes.TypedData, error) {
	bz, err := json.Marshal(td)
	if err != nil {
		return apitypes.TypedData{}, fmt.Errorf("marshal typed data: %w", err)
	}
	var out apitypes.TypedData
	if err := json.Unmarshal(bz, &out); err != nil {
		return apitypes.TypedData{}, fmt.Errorf("decode typed data: %w", err)
	}
	// Ensure domain chainId is populated for apitypes even when the wire form used a JSON number.
	if out.Domain.ChainId == nil {
		if id, err := parseDomainChainID(td.Domain.ChainId); err == nil {
			out.Domain.ChainId = (*math.HexOrDecimal256)(new(big.Int).SetUint64(id))
		}
	}
	return out, nil
}
