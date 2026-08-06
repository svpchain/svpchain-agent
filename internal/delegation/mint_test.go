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

// A default (non-redelegable) credential must carry an explicitly constrained
// empty redelegate_to — never the zero OptionalStringSet, whose unconstrained
// form is permissive — and a max_depth of 1.
func TestMintDefaultsAreNonRedelegable(t *testing.T) {
	l := mintFixture(t, "1", false)
	proof, err := l.Mint(context.Background(), baseParams())
	require.NoError(t, err)

	cav := mustToken(t, proof[0]).Payload.Caveats
	require.False(t, cav.Redelegable)
	require.Equal(t, uint32(1), cav.MaxDepth)
	require.True(t, cav.RedelegateTo.Constrained, "redelegate_to must never be the permissive unconstrained form")
	require.Empty(t, cav.RedelegateTo.Set)
}

func TestMintRedelegableCaveats(t *testing.T) {
	const executorDID = "did:svp:svp1executor"
	l := mintFixture(t, "1", false)
	p := baseParams()
	p.Redelegable = true
	p.RedelegateTo = []string{executorDID}
	proof, err := l.Mint(context.Background(), p)
	require.NoError(t, err)

	cav := mustToken(t, proof[0]).Payload.Caveats
	require.True(t, cav.Redelegable)
	require.Equal(t, uint32(2), cav.MaxDepth)
	require.True(t, cav.RedelegateTo.Constrained)
	require.True(t, cav.RedelegateTo.Set.Has(executorDID))
}

func TestMintRedelegableValidation(t *testing.T) {
	l := mintFixture(t, "1", false)

	p := baseParams()
	p.Redelegable = true // no targets
	_, err := l.Mint(context.Background(), p)
	require.ErrorContains(t, err, "redelegate_to")

	p = baseParams()
	p.RedelegateTo = []string{"did:svp:svp1executor"} // targets without redelegable
	_, err = l.Mint(context.Background(), p)
	require.ErrorContains(t, err, "not redelegable")

	p = baseParams()
	p.Redelegable = true
	p.RedelegateTo = []string{"svp1notadid"}
	_, err = l.Mint(context.Background(), p)
	require.ErrorContains(t, err, "did:svp:")
}

// The intermediary topology end to end: the user mints a redelegable
// credential to an intermediary, the intermediary attenuates it to the named
// executor, and the 2-token chain verifies for the executor — while
// attenuating to anyone outside redelegate_to is refused.
func TestMintRedelegableChainAttenuatesToNamedExecutor(t *testing.T) {
	const (
		interDID    = "did:svp:svp1intermediary"
		executorDID = "did:svp:svp1executor"
	)
	l := mintFixture(t, "1", false)

	p := baseParams()
	p.AudienceDID = interDID
	p.Redelegable = true
	p.RedelegateTo = []string{executorDID}
	proof, err := l.Mint(context.Background(), p)
	require.NoError(t, err)
	parentRaw, err := base64.StdEncoding.DecodeString(proof[0])
	require.NoError(t, err)
	parent := mustToken(t, proof[0])

	interKey := make([]byte, 32)
	interKey[31] = 0x42
	interSigner, err := svpdt.NewPrivateKeySigner(interKey)
	require.NoError(t, err)

	childCaveats := parent.Payload.Caveats.Clone()
	childCaveats.Redelegable = false
	childCaveats.RedelegateTo, err = svpdt.ConstrainedTo()
	require.NoError(t, err)

	// Outside the target list: the cooperative path already refuses.
	_, _, err = svpdt.Attenuate(interSigner, svpdt.AttenuateParams{
		Parent:   parentRaw,
		Issuer:   interDID,
		Audience: "did:svp:svp1someoneelse",
		Caveats:  childCaveats,
		Nonce:    [16]byte{0x07},
	})
	require.Error(t, err, "attenuating outside redelegate_to must refuse")

	_, childRaw, err := svpdt.Attenuate(interSigner, svpdt.AttenuateParams{
		Parent:   parentRaw,
		Issuer:   interDID,
		Audience: executorDID,
		Caveats:  childCaveats,
		Nonce:    [16]byte{0x08},
	})
	require.NoError(t, err)

	resolver := svpdt.SingleKeyResolver(map[string][]byte{
		l.OwnerDID(): l.Priv.PubKey().Bytes(),
		interDID:     interSigner.PublicKey(),
	})
	verified, err := svpdt.VerifyChain([][]byte{parentRaw, childRaw}, resolver, svpdt.VerifyOpts{
		ChainID:  testChainID,
		Now:      testNow,
		MaxDepth: 4,
		Audience: executorDID,
	})
	require.NoError(t, err)
	require.Equal(t, l.Owner(), verified.Principal, "the principal survives the extra hop")
	require.Equal(t, uint32(2), verified.Depth)
}
