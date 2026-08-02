package delegation

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/stretchr/testify/require"
	"github.com/svpchain/svpdt"

	"github.com/svpchain/svpchain-local-agent/internal/registry"
)

const (
	testChainID = "svp-mint-test"
	testNow     = int64(1_700_000_000)
	audienceDID = "did:svp:svp1agentoperator"
)

func mintFixture(t *testing.T, epoch string, paused bool) *Lifecycle {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/dydxprotocol/agentwallet/epoch/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"epoch":%q,"paused":%v}`, epoch, paused)
	})
	mux.HandleFunc("/dydxprotocol/agentwallet/params", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"params":{"max_delegation_depth":4,"max_token_ttl_seconds":"120"}}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	priv, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)

	return &Lifecycle{
		Registry: registry.New(srv.URL),
		Priv:     priv,
		ChainID:  testChainID,
	}
}

func baseParams() MintParams {
	rootID := make([]byte, 32)
	rootID[0] = 0x01
	return MintParams{
		RootID:      rootID,
		AudienceDID: audienceDID,
		Actions:     []string{"clob.place_order"},
		Subaccounts: []uint32{0},
		Budget:      []registry.Coin{{Denom: "uusdc", Amount: "1000000"}},
		Now:         testNow,
	}
}

// The whole point: what Mint produces must verify as a user-issued root
// credential — signed by the user's account key under their own DID.
func TestMintRoundTripsThroughVerifyChain(t *testing.T) {
	l := mintFixture(t, "3", false)
	proof, err := l.Mint(context.Background(), baseParams())
	require.NoError(t, err)
	require.Len(t, proof, 1)

	raw, err := base64.StdEncoding.DecodeString(proof[0])
	require.NoError(t, err)

	resolver := svpdt.SingleKeyResolver(map[string][]byte{
		l.OwnerDID(): l.Priv.PubKey().Bytes(),
	})
	verified, err := svpdt.VerifyChain([][]byte{raw}, resolver, svpdt.VerifyOpts{
		ChainID:  testChainID,
		Now:      testNow,
		MaxDepth: 4,
		Audience: audienceDID,
	})
	require.NoError(t, err)

	require.Equal(t, l.Owner(), verified.Principal)
	require.Equal(t, uint64(3), verified.RootEpoch)
	require.True(t, verified.Effective.Actions.Has("clob.place_order"))
	require.True(t, verified.Effective.Subaccounts.Has(0))
	require.False(t, verified.Effective.Actions.Has("clob.cancel_order"))
}

// The requested TTL must clamp to the chain ceiling (120s in the fixture).
func TestMintClampsTTLToChainCeiling(t *testing.T) {
	l := mintFixture(t, "1", false)
	p := baseParams()
	p.TTLSeconds = 3600
	proof, err := l.Mint(context.Background(), p)
	require.NoError(t, err)

	raw, err := base64.StdEncoding.DecodeString(proof[0])
	require.NoError(t, err)
	tok, err := svpdt.UnmarshalToken(raw)
	require.NoError(t, err)
	require.Equal(t, testNow+120, tok.Payload.Caveats.Expires)
}

// A paused delegation must refuse to mint at all — not mint a doomed token.
func TestMintRefusesWhilePaused(t *testing.T) {
	l := mintFixture(t, "2", true)
	_, err := l.Mint(context.Background(), baseParams())
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "paused"))
}

// Two mints never share a nonce: every credential is single-use on chain, so
// a reused nonce would make the second task fail as a replay.
func TestMintDrawsFreshNonces(t *testing.T) {
	l := mintFixture(t, "1", false)
	first, err := l.Mint(context.Background(), baseParams())
	require.NoError(t, err)
	second, err := l.Mint(context.Background(), baseParams())
	require.NoError(t, err)

	tok1 := mustToken(t, first[0])
	tok2 := mustToken(t, second[0])
	require.NotEqual(t, tok1.Payload.Nonce, tok2.Payload.Nonce)
}

func mustToken(t *testing.T, b64 string) *svpdt.Token {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err)
	tok, err := svpdt.UnmarshalToken(raw)
	require.NoError(t, err)
	return tok
}
