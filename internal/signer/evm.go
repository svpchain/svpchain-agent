package signer

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"

	"github.com/svpchain/svpchain-agent/internal/payload"
)

// DeriveEvmAddress returns the 0x-checksummed Ethereum address derived from
// priv's public key. eth_secp256k1 keys derive the same 20-byte address the
// EVM uses (keccak256(pubkey)[12:]), so this is the EVM-side counterpart of
// DeriveAddress's bech32 svp1… form — both name the same account.
func DeriveEvmAddress(priv *ethsecp256k1.PrivKey) string {
	return common.BytesToAddress(priv.PubKey().Address()).Hex()
}

// EvmToBech32 re-encodes a 0x Ethereum address as its svpchain bech32 (svp1…)
// form. An eth_secp256k1 account has a single 20-byte identity shared by both
// encodings (see DeriveEvmAddress), so this is a pure address conversion — it
// needs no private key and the address need not belong to the loaded key. Use
// it to turn an EVM recipient into the svp1… address a Cosmos x/bank send
// expects, e.g. to send SVP to a 0x address.
//
// The input may be checksummed or lowercase, with or without the 0x prefix;
// non-address input is rejected rather than silently re-encoded.
func EvmToBech32(evmAddr string) (string, error) {
	s := strings.TrimSpace(evmAddr)
	if !common.IsHexAddress(s) {
		return "", fmt.Errorf("%q is not a valid 0x Ethereum address", evmAddr)
	}
	return sdk.AccAddress(common.HexToAddress(s).Bytes()).String(), nil
}

// SignEvm builds a raw Ethereum transaction from p, signs it with priv, and
// returns the canonical RLP-encoded signed transaction (hex) ready for
// eth_sendRawTransaction. Supports EIP-1559 (type 2) and legacy txs.
//
// Cross-checks (mirror of Sign for the Cosmos path):
//   - p.Version must equal payload.CurrentVersion.
//   - If p.SignerAddress is non-empty it must equal the key-derived 0x
//     address; empty is tolerated for ad-hoc demos.
//
// The EVM chain-id replay guard (payload chain id == the signer's bound chain)
// lives in the handler, before this is called, alongside the Cosmos one.
func SignEvm(priv *ethsecp256k1.PrivKey, p *payload.EvmTxPayload) (*payload.SignedEvmTx, error) {
	if p.Version != payload.CurrentVersion {
		return nil, fmt.Errorf("unsupported EvmTxPayload version %d (want %d)", p.Version, payload.CurrentVersion)
	}

	keyAddr := common.BytesToAddress(priv.PubKey().Address())
	if p.SignerAddress != "" {
		if !common.IsHexAddress(p.SignerAddress) {
			return nil, fmt.Errorf("payload.signer_address %q is not a valid 0x address", p.SignerAddress)
		}
		if common.HexToAddress(p.SignerAddress) != keyAddr {
			return nil, fmt.Errorf("key-derived signer address %s does not match payload.signer_address %s",
				keyAddr.Hex(), p.SignerAddress)
		}
	}

	chainID, err := parseBigInt(p.EVMChainID, "evm_chain_id")
	if err != nil {
		return nil, err
	}
	if chainID.Sign() <= 0 {
		return nil, fmt.Errorf("evm_chain_id must be positive, got %q", p.EVMChainID)
	}
	nonce, err := strconv.ParseUint(p.Nonce, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse nonce %q: %w", p.Nonce, err)
	}
	gas, err := strconv.ParseUint(p.Gas, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse gas %q: %w", p.Gas, err)
	}
	value, err := parseOptionalBigInt(p.Value, "value")
	if err != nil {
		return nil, err
	}
	to, err := parseTo(p.To)
	if err != nil {
		return nil, err
	}
	data, err := parseData(p.Data)
	if err != nil {
		return nil, err
	}

	inner, err := buildTxData(p, chainID, nonce, gas, value, to, data)
	if err != nil {
		return nil, err
	}

	ecdsaKey, err := ethcrypto.ToECDSA(priv.Key)
	if err != nil {
		return nil, fmt.Errorf("convert key to ecdsa: %w", err)
	}
	signed, err := ethtypes.SignTx(ethtypes.NewTx(inner), ethtypes.LatestSignerForChainID(chainID), ecdsaKey)
	if err != nil {
		return nil, fmt.Errorf("sign evm tx: %w", err)
	}
	rawBytes, err := signed.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("encode signed evm tx: %w", err)
	}

	v, r, s := signed.RawSignatureValues()
	return &payload.SignedEvmTx{
		RawTxHex: hexutil.Encode(rawBytes),
		TxHash:   signed.Hash().Hex(),
		V:        v.String(),
		R:        r.String(),
		S:        s.String(),
	}, nil
}

// buildTxData assembles the format-specific go-ethereum TxData. The tx type is
// taken from p.TxType, inferred when empty: a populated max_fee_per_gas means
// EIP-1559, otherwise legacy.
func buildTxData(
	p *payload.EvmTxPayload,
	chainID *big.Int,
	nonce, gas uint64,
	value *big.Int,
	to *common.Address,
	data []byte,
) (ethtypes.TxData, error) {
	txType := p.TxType
	if txType == "" {
		if p.MaxFeePerGas != "" {
			txType = payload.EVMTxTypeEIP1559
		} else {
			txType = payload.EVMTxTypeLegacy
		}
	}

	switch txType {
	case payload.EVMTxTypeEIP1559:
		gasFeeCap, err := parseBigInt(p.MaxFeePerGas, "max_fee_per_gas")
		if err != nil {
			return nil, err
		}
		gasTipCap, err := parseOptionalBigInt(p.MaxPriorityFeePerGas, "max_priority_fee_per_gas")
		if err != nil {
			return nil, err
		}
		return &ethtypes.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			GasTipCap: gasTipCap,
			GasFeeCap: gasFeeCap,
			Gas:       gas,
			To:        to,
			Value:     value,
			Data:      data,
		}, nil

	case payload.EVMTxTypeLegacy:
		gasPrice, err := parseBigInt(p.GasPrice, "gas_price")
		if err != nil {
			return nil, err
		}
		return &ethtypes.LegacyTx{
			Nonce:    nonce,
			GasPrice: gasPrice,
			Gas:      gas,
			To:       to,
			Value:    value,
			Data:     data,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported tx_type %q (want %q or %q)",
			txType, payload.EVMTxTypeEIP1559, payload.EVMTxTypeLegacy)
	}
}

// parseBigInt parses a required decimal string into a big.Int.
func parseBigInt(s, field string) (*big.Int, error) {
	if strings.TrimSpace(s) == "" {
		return nil, fmt.Errorf("%s is required", field)
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("parse %s %q: not a base-10 integer", field, s)
	}
	if v.Sign() < 0 {
		return nil, fmt.Errorf("%s must not be negative, got %q", field, s)
	}
	return v, nil
}

// parseOptionalBigInt parses a decimal string into a big.Int, treating empty
// as zero.
func parseOptionalBigInt(s, field string) (*big.Int, error) {
	if strings.TrimSpace(s) == "" {
		return new(big.Int), nil
	}
	return parseBigInt(s, field)
}

// parseTo parses the recipient address, returning nil for an empty string
// (contract creation).
func parseTo(s string) (*common.Address, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	if !common.IsHexAddress(s) {
		return nil, fmt.Errorf("to %q is not a valid 0x address", s)
	}
	a := common.HexToAddress(s)
	return &a, nil
}

// parseData decodes 0x-hex call data, returning nil for an empty string.
func parseData(s string) ([]byte, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	b, err := hexutil.Decode(s)
	if err != nil {
		return nil, fmt.Errorf("decode data: %w", err)
	}
	return b, nil
}
