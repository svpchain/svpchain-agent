package signer

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/cosmos/gogoproto/proto"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"

	appconfig "github.com/svpchain/svpchain-agent/internal/config"
	"github.com/svpchain/svpchain-agent/internal/payload"
)

func init() {
	// Set svp bech32 prefixes so sdk.AccAddress stringification matches the chain.
	// Import this package — callers need not blank-import internal/config.
	appconfig.SetAddressPrefixes()
}

// ParsePrivKey decodes a 32-byte eth_secp256k1 private key from a hex string.
// Leading "0x" is allowed; surrounding whitespace is trimmed.
func ParsePrivKey(s string) (*ethsecp256k1.PrivKey, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	bz, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("hex decode: %w", err)
	}
	if len(bz) != 32 {
		return nil, fmt.Errorf("private key must be 32 bytes (got %d)", len(bz))
	}
	return &ethsecp256k1.PrivKey{Key: bz}, nil
}

// DeriveAddress derives a bech32 address string (svp prefix) from priv's public key.
func DeriveAddress(priv *ethsecp256k1.PrivKey) string {
	return sdk.AccAddress(priv.PubKey().Address()).String()
}

// DeriveEVMAddress derives an Ethereum-format address (0x prefix, 40 hex chars) from priv.
func DeriveEVMAddress(priv *ethsecp256k1.PrivKey) string {
	key, err := priv.ToECDSA()
	if err != nil {
		return ""
	}
	return gethcrypto.PubkeyToAddress(key.PublicKey).Hex()
}

// Sign turns p into a SignedTx: builds AuthInfo with the signer's pubkey, computes SIGN_MODE_DIRECT sign bytes
// via payload.DirectSignBytes (same layout as the remote MCP server), signs with priv, and proto-serializes TxRaw.
//
// Cross-checks:
//   - p.Version must equal payload.CurrentVersion
//   - if p.SignerAddress is non-empty it must match the key-derived address; empty is allowed (temporary demos —
//     callers bear mismatch risk)
func Sign(priv *ethsecp256k1.PrivKey, p *payload.TxPayload) (*payload.SignedTx, error) {
	if p.Version != payload.CurrentVersion {
		return nil, fmt.Errorf("unsupported TxPayload version %d (want %d)", p.Version, payload.CurrentVersion)
	}

	pub := priv.PubKey()
	signerAddr := sdk.AccAddress(pub.Address()).String()
	if p.SignerAddress != "" && signerAddr != p.SignerAddress {
		return nil, fmt.Errorf("key-derived signer address %s does not match payload.signer_address %s",
			signerAddr, p.SignerAddress)
	}

	accNum, err := strconv.ParseUint(p.AccountNumber, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse account_number %q: %w", p.AccountNumber, err)
	}
	seq, err := strconv.ParseUint(p.Sequence, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse sequence %q: %w", p.Sequence, err)
	}
	gasLimit, err := strconv.ParseUint(p.Fee.GasLimit, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse fee.gas_limit %q: %w", p.Fee.GasLimit, err)
	}
	feeAmount, err := toSDKCoins(p.Fee.Amount)
	if err != nil {
		return nil, err
	}

	pubAny, err := codectypes.NewAnyWithValue(pub)
	if err != nil {
		return nil, fmt.Errorf("wrap pubkey in Any: %w", err)
	}

	authInfo := &txtypes.AuthInfo{
		SignerInfos: []*txtypes.SignerInfo{{
			PublicKey: pubAny,
			ModeInfo: &txtypes.ModeInfo{
				Sum: &txtypes.ModeInfo_Single_{
					Single: &txtypes.ModeInfo_Single{
						Mode: signing.SignMode_SIGN_MODE_DIRECT,
					},
				},
			},
			Sequence: seq,
		}},
		Fee: &txtypes.Fee{
			// Honor payload.Fee.Amount: zero-fee short-term CLOB txs have empty Amount (feeAmount nil);
			// other txs fill from the payload. Fees come from the remote MCP server builder, not the signer.
			Amount:   feeAmount,
			GasLimit: gasLimit,
		},
	}
	authInfoBytes, err := proto.Marshal(authInfo)
	if err != nil {
		return nil, fmt.Errorf("marshal AuthInfo: %w", err)
	}

	// TxPayload.TxBodyBytesB64 is a base64 string on the wire (see payload.TxPayload); decode before signing.
	bodyBytes, err := base64.StdEncoding.DecodeString(p.TxBodyBytesB64)
	if err != nil {
		return nil, fmt.Errorf("decode tx_body_bytes_b64: %w", err)
	}
	signBytes, err := payload.DirectSignBytes(bodyBytes, authInfoBytes, p.ChainID, accNum)
	if err != nil {
		return nil, fmt.Errorf("compute sign-bytes: %w", err)
	}
	sig, err := priv.Sign(signBytes)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	txRaw := &txtypes.TxRaw{
		BodyBytes:     bodyBytes,
		AuthInfoBytes: authInfoBytes,
		Signatures:    [][]byte{sig},
	}
	txRawBytes, err := proto.Marshal(txRaw)
	if err != nil {
		return nil, fmt.Errorf("marshal TxRaw: %w", err)
	}

	return &payload.SignedTx{
		TxRawBytesB64: base64.StdEncoding.EncodeToString(txRawBytes),
		SignatureB64:  base64.StdEncoding.EncodeToString(sig),
		PubKeyB64:     base64.StdEncoding.EncodeToString(pub.Bytes()),
	}, nil
}

// toSDKCoins converts the wire-form fee coins into sdk.Coins for AuthInfo.Fee.
// An empty input yields nil (the zero-fee form short-term CLOB txs rely on),
// so the signer simply honors whatever fee the payload carries rather than
// hardcoding it. Amounts are parsed as integers to preserve full precision.
func toSDKCoins(coins []payload.Coin) (sdk.Coins, error) {
	if len(coins) == 0 {
		return nil, nil
	}
	out := make(sdk.Coins, 0, len(coins))
	for _, c := range coins {
		amt, ok := math.NewIntFromString(c.Amount)
		if !ok {
			return nil, fmt.Errorf("parse fee.amount %q for denom %q", c.Amount, c.Denom)
		}
		// Construct the coin directly (sdk.NewCoin panics on invalid input);
		// Validate surfaces a bad denom or negative amount as an error since
		// the signer may receive payloads it didn't build.
		coin := sdk.Coin{Denom: c.Denom, Amount: amt}
		if err := coin.Validate(); err != nil {
			return nil, fmt.Errorf("invalid fee coin %s%s: %w", c.Amount, c.Denom, err)
		}
		out = append(out, coin)
	}
	// Canonical order is required for the chain to accept the fee; Sort also
	// makes the result a valid sdk.Coins.
	return out.Sort(), nil
}
