package payload

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/gogoproto/proto"
)

// DirectSignBytes returns SIGN_MODE_DIRECT sign bytes for a Cosmos transaction:
// proto-serialized cosmos.tx.v1beta1.SignDoc with body, auth-info, chain id, and account number.
//
// Matches the byte sequence from the standard on-chain SDK tx.SignWithPrivKey path;
// this only produces bytes to sign — the local signer signs them and returns them via broadcast_signed_tx.
func DirectSignBytes(bodyBytes, authInfoBytes []byte, chainID string, accountNumber uint64) ([]byte, error) {
	signDoc := &tx.SignDoc{
		BodyBytes:     bodyBytes,
		AuthInfoBytes: authInfoBytes,
		ChainId:       chainID,
		AccountNumber: accountNumber,
	}
	out, err := proto.Marshal(signDoc)
	if err != nil {
		return nil, fmt.Errorf("marshal SignDoc: %w", err)
	}
	return out, nil
}
