package registry

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveEndpoint exercises the client against a real chain REST gateway,
// which is where encoding surprises live: grpc-gateway renders uint64 as
// JSON strings, bytes as base64, and enums as names, and a stub server only
// proves the client agrees with the stub.
//
// Opt-in, because it needs the network and a running node:
//
//	CHAIN_REST_URL=https://dev02.svpchain.org go test ./internal/registry/ -run TestLive -v
func TestLiveEndpoint(t *testing.T) {
	base := os.Getenv("CHAIN_REST_URL")
	if base == "" {
		t.Skip("set CHAIN_REST_URL to run against a live chain")
	}
	c := New(base)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	params, err := c.WalletParams(ctx)
	if err != nil {
		t.Fatalf("wallet params: %v", err)
	}
	if params.MaxDelegationDepth == 0 || params.MaxTokenTtlSeconds == 0 {
		t.Fatalf("params look unpopulated: %+v", params)
	}
	t.Logf("agentwallet params: max_depth=%d max_ttl=%ds max_proof_tokens=%d",
		params.MaxDelegationDepth, params.MaxTokenTtlSeconds, params.MaxProofTokens)

	agents, err := c.Agents(ctx, "")
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	t.Logf("registered ACTIVE agents: %d", len(agents))
	for _, a := range agents {
		t.Logf("  %s endpoint=%q capabilities=%v key=%dB",
			a.AgentID, a.Endpoint, a.Capabilities, len(a.PublicKey))
		if len(a.PublicKey) != 33 {
			t.Errorf("%s: public key is %d bytes, want 33 — base64 decoding is wrong",
				a.AgentID, len(a.PublicKey))
		}
		card, err := c.FetchCard(ctx, a)
		if err != nil {
			t.Logf("  card unreachable: %v", err)
			continue
		}
		t.Logf("  card verified against on-chain hash: %v", card.Verified)
	}
}

// TestLiveAccount checks the account parser against real gateway JSON, where
// account_number arrives as a string and non-BaseAccount types nest the
// fields one level down.
func TestLiveAccount(t *testing.T) {
	base := os.Getenv("CHAIN_REST_URL")
	if base == "" {
		t.Skip("set CHAIN_REST_URL to run against a live chain")
	}
	addr := os.Getenv("CHAIN_TEST_ADDRESS")
	if addr == "" {
		t.Skip("set CHAIN_TEST_ADDRESS to an account that exists on that chain")
	}
	c := New(base)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := c.Account(ctx, addr)
	if err != nil {
		t.Fatalf("account %s: %v", addr, err)
	}
	t.Logf("account %s: number=%d sequence=%d", addr, info.AccountNumber, info.Sequence)
	if info.AccountNumber == 0 {
		t.Errorf("account_number parsed as 0 — likely a string/number decoding bug")
	}
}
