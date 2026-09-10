package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/stretchr/testify/require"

	localsigner "github.com/svpchain/svpchain-agent/internal/agent/local"
)

// testSigner is a real local signer over a throwaway key: callWithReauth only
// needs Owner and SignChallenge to exist, never a particular address.
func testSigner(t *testing.T) *localsigner.Signer {
	t.Helper()
	priv, err := ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	return localsigner.NewSigner(priv, "svp_2517-1", 2517)
}

// The exact reply the svpchain services return for a tenant-scoped tool called
// without a bearer -- a SUCCESSFUL result, not an error.
const authRequiredReply = "authentication required: this tool needs a bearer token. Complete the handshake, then retry:\n" +
	"  1) call the svpchain-signer MCP server's whoami tool to get your svp1… owner address\n" +
	"  2) call auth_challenge with that owner address"

// fakeRetrier is a transport that refuses until the handshake has run.
type fakeRetrier struct {
	calls        int
	invalidated  int
	handshakes   int
	handshakeErr error
	authed       bool
}

func (f *fakeRetrier) InvalidateBearer() { f.invalidated++ }

func (f *fakeRetrier) EnsureAuth(_ context.Context, _ string, _ func(string) (string, error)) error {
	f.handshakes++
	if f.handshakeErr != nil {
		return f.handshakeErr
	}
	f.authed = true
	return nil
}

// call answers auth_required until authenticated, as a soft result.
func (f *fakeRetrier) call(context.Context) (string, error) {
	f.calls++
	if !f.authed {
		return authRequiredReply, nil
	}
	return `{"ok":true}`, nil
}

func TestIsAuthRequired(t *testing.T) {
	require.True(t, isAuthRequired(authRequiredReply, nil), "soft result must be detected")
	require.True(t, isAuthRequired("", fmt.Errorf("%s", authRequiredReply)), "hard error must be detected")
	require.False(t, isAuthRequired(`{"ok":true}`, nil))
	require.False(t, isAuthRequired("", fmt.Errorf("insufficient funds")))
}

// The point of the change: a tool answering auth_required drives the handshake
// itself, rather than depending on the model noticing prose in the result.
func TestCallWithReauthRunsHandshakeAndRetries(t *testing.T) {
	transport := &fakeRetrier{}
	env := dispatchEnv{local: testSigner(t)}

	out, err := env.callWithReauth(context.Background(), "get_balance", transport, transport.call)
	require.NoError(t, err)
	require.Equal(t, `{"ok":true}`, out, "the retry's result is what the caller sees")
	require.Equal(t, 2, transport.calls, "called once, then once more after authenticating")
	require.Equal(t, 1, transport.handshakes)
}

// EnsureAuth short-circuits on a cached bearer, so a token the service has
// already refused would make the retry fail exactly as the first call did.
func TestCallWithReauthInvalidatesTheRefusedBearer(t *testing.T) {
	transport := &fakeRetrier{}
	env := dispatchEnv{local: testSigner(t)}

	_, err := env.callWithReauth(context.Background(), "get_balance", transport, transport.call)
	require.NoError(t, err)
	require.Equal(t, 1, transport.invalidated, "the refused bearer must be dropped before re-authenticating")
}

// Exactly once. A service that answers auth_required no matter what must not
// spin the run.
func TestCallWithReauthRetriesOnlyOnce(t *testing.T) {
	calls := 0
	alwaysRefuses := func(context.Context) (string, error) {
		calls++
		return authRequiredReply, nil
	}
	transport := &fakeRetrier{}
	env := dispatchEnv{local: testSigner(t)}

	out, err := env.callWithReauth(context.Background(), "get_balance", transport, alwaysRefuses)
	require.NoError(t, err)
	require.Equal(t, authRequiredReply, out, "the service's own answer survives, handshake steps included")
	require.Equal(t, 2, calls, "one retry, never a loop")
}

// The handshake cannot authenticate itself; retrying it would recurse.
func TestCallWithReauthSkipsHandshakeTools(t *testing.T) {
	for _, name := range []string{"auth_challenge", "auth_verify"} {
		t.Run(name, func(t *testing.T) {
			transport := &fakeRetrier{}
			env := dispatchEnv{local: testSigner(t)}

			out, err := env.callWithReauth(context.Background(), name, transport, transport.call)
			require.NoError(t, err)
			require.Equal(t, authRequiredReply, out)
			require.Equal(t, 1, transport.calls)
			require.Zero(t, transport.handshakes, "the handshake must not try to authenticate itself")
		})
	}
}

// No key, no challenge to sign. The service's instructions must survive so the
// model can still complete the handshake as a tool sequence.
func TestCallWithReauthWithoutLocalSigner(t *testing.T) {
	transport := &fakeRetrier{}

	out, err := dispatchEnv{}.callWithReauth(context.Background(), "get_balance", transport, transport.call)
	require.NoError(t, err)
	require.Equal(t, authRequiredReply, out)
	require.Zero(t, transport.handshakes)
}

// A failed handshake must not replace the service's actionable instructions
// with an internal error.
func TestCallWithReauthFallsBackWhenHandshakeFails(t *testing.T) {
	transport := &fakeRetrier{handshakeErr: fmt.Errorf("signer unavailable")}
	env := dispatchEnv{local: testSigner(t)}

	out, err := env.callWithReauth(context.Background(), "get_balance", transport, transport.call)
	require.NoError(t, err)
	require.Contains(t, out, authRequiredMarker)
	require.Equal(t, 1, transport.calls, "no retry after a failed handshake")
	require.Equal(t, 1, transport.handshakes)
}

// A transport with no handshake support (or none attached) passes the answer
// through untouched rather than erroring.
func TestCallWithReauthWithoutRetrier(t *testing.T) {
	calls := 0
	env := dispatchEnv{local: testSigner(t)}

	out, err := env.callWithReauth(context.Background(), "get_balance", nil, func(context.Context) (string, error) {
		calls++
		return authRequiredReply, nil
	})
	require.NoError(t, err)
	require.Equal(t, authRequiredReply, out)
	require.Equal(t, 1, calls)
}

// An ordinary result must not pay for any of this.
func TestCallWithReauthLeavesNormalResultsAlone(t *testing.T) {
	transport := &fakeRetrier{authed: true}
	env := dispatchEnv{local: testSigner(t)}

	out, err := env.callWithReauth(context.Background(), "get_balance", transport, transport.call)
	require.NoError(t, err)
	require.Equal(t, `{"ok":true}`, out)
	require.Equal(t, 1, transport.calls)
	require.Zero(t, transport.handshakes)
	require.Zero(t, transport.invalidated)
}

// fakeAttachedAgent is an attached A2A agent as it behaves at the start of an
// attachment: it holds no bearer, so it refuses a tenant-scoped call with the
// service's handshake instructions. a2amcp turns a non-OK reply into an error,
// which is why this refuses as an error rather than as a soft result.
type fakeAttachedAgent struct {
	fakeSource
	authed     bool
	handshakes int
}

func (f *fakeAttachedAgent) InvalidateBearer() { f.authed = false }

func (f *fakeAttachedAgent) EnsureAuth(_ context.Context, _ string, _ func(string) (string, error)) error {
	f.handshakes++
	f.authed = true
	return nil
}

func (f *fakeAttachedAgent) CallTool(_ context.Context, name string, _ map[string]any) (string, error) {
	f.called = append(f.called, name)
	if !f.authed {
		return "", fmt.Errorf("%s", authRequiredReply)
	}
	return `{"ok":true}`, nil
}

// Every route to an attached agent must authenticate on first use. The paid
// resume path used to call the agent directly, so the first tool call of a
// conversation that renewed its settlement died on "authenticate first" — and a
// tool error ends the run, leaving the model no round in which to recover.
func TestCallAttachedRunsHandshakeOnFirstUse(t *testing.T) {
	remote := &fakeAttachedAgent{fakeSource: fakeSource{tools: []string{"build_place_market_order"}}}
	att := newAttached(nil)
	require.Equal(t, []string{"build_place_market_order"}, att.set(remote))
	env := dispatchEnv{local: testSigner(t), att: att}

	out, err := env.callAttached(context.Background(), "build_place_market_order", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, `{"ok":true}`, out)
	require.Equal(t, 1, remote.handshakes)
	require.Equal(t, []string{"build_place_market_order", "build_place_market_order"}, remote.called)
}
