package delegation

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/registry"
)

// TestEmitCrossRepoFixture regenerates the credential fixture the chain's
// keeper test verifies (svpagent x/agentwallet/keeper). Skipped unless
// EMIT_CROSSREPO_FIXTURE is set; it is a generator, not an assertion.
func TestEmitCrossRepoFixture(t *testing.T) {
	if os.Getenv("EMIT_CROSSREPO_FIXTURE") == "" {
		t.Skip("set EMIT_CROSSREPO_FIXTURE=1 to regenerate")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/dydxprotocol/agentwallet/epoch/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"epoch":"1","paused":false}`)
	})
	mux.HandleFunc("/dydxprotocol/agentwallet/params", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"params":{"max_delegation_depth":4,"max_token_ttl_seconds":"600"}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	key := make([]byte, 32)
	key[31] = 0x6B
	life := &Lifecycle{
		Registry: registry.New(srv.URL),
		Priv:     &ethsecp256k1.PrivKey{Key: key},
		ChainID:  "svp-delegation-test",
	}

	rootID := mustHex(t, "518e806512f0e1614c9f1fea2ad61b06adb3468f9d921a6c178b71bb28872cb8")
	proof, err := life.Mint(context.Background(), MintParams{
		RootID:      rootID,
		AudienceDID: "did:svp:svp1j2gxfxa9yz34jyk6k9enkmcfskr7gvh06033tw",
		Actions:     []string{"clob.place_order"},
		Subaccounts: []uint32{7},
		// The chain prices the fixture's order at 10 of the quote denom; the
		// credential must carry a budget that covers it.
		Budget:     []registry.Coin{{Denom: "erc20/usdc", Amount: "50"}},
		TTLSeconds: 300,
		Now:        1_700_000_000,
	})
	require.NoError(t, err)

	fmt.Println("OWNER:", life.Owner())
	fmt.Println("PROOF_B64:", proof[0])
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		var v int
		_, err := fmt.Sscanf(s[i*2:i*2+2], "%02x", &v)
		require.NoError(t, err)
		out[i] = byte(v)
	}
	return out
}
