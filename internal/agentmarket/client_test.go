package agentmarket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/agents/did:svp:evm", r.URL.Path)
		_, _ = w.Write([]byte(`{"agent_id":"did:svp:evm","owner":"svp1owner","endpoint":"http://agent","status":"AGENT_STATUS_ACTIVE","pricing":{"amount":"1000000","unit":"call"}}`))
	}))
	t.Cleanup(server.Close)

	hit, err := New(server.URL).Get(context.Background(), "did:svp:evm")
	require.NoError(t, err)
	require.Equal(t, "svp1owner", hit.Owner)
	require.Equal(t, "1000000", hit.Pricing.Amount)
}

func TestGetDecodesStructuredCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"agent_id":"did:svp:evm","capabilities":{"categories":["LENDING","EVM"],"tags":["lendora","supply"]}}`))
	}))
	t.Cleanup(server.Close)

	hit, err := New(server.URL).Get(context.Background(), "did:svp:evm")
	require.NoError(t, err)
	require.Equal(t, []string{"LENDING", "EVM"}, hit.Capabilities.Categories)
	require.Equal(t, []string{"lendora", "supply"}, hit.Capabilities.Tags)
}

func TestGetDecodesLegacyCapabilitiesAsTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"agent_id":"did:svp:evm","capabilities":["evm.swap"]}`))
	}))
	t.Cleanup(server.Close)

	hit, err := New(server.URL).Get(context.Background(), "did:svp:evm")
	require.NoError(t, err)
	require.Empty(t, hit.Capabilities.Categories)
	require.Equal(t, []string{"evm.swap"}, hit.Capabilities.Tags)
}

func TestFindActiveByEndpointScansPagesAndNormalizesTrailingSlash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/agents", r.URL.Path)
		switch r.URL.Query().Get("cursor") {
		case "0":
			_, _ = w.Write([]byte(`{"agents":[{"agent_id":"did:svp:first","endpoint":"https://first.example"}],"cursor":0,"next_cursor":1}`))
		case "1":
			_, _ = w.Write([]byte(`{"agents":[{"agent_id":"did:svp:target","endpoint":"https://target.example/a2a/","status":"AGENT_STATUS_ACTIVE"}],"cursor":1,"next_cursor":2}`))
		default:
			t.Fatalf("unexpected cursor %q", r.URL.Query().Get("cursor"))
		}
	}))
	t.Cleanup(server.Close)

	hit, found, err := New(server.URL).FindActiveByEndpoint(context.Background(), " https://target.example/a2a ")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "did:svp:target", hit.AgentID)
	require.Equal(t, "https://target.example/a2a", hit.Endpoint)
}

// testCard is a small Agent Card together with the base64 SHA-256 of these
// exact bytes — the value an owner would commit on chain. Hardcoded rather than
// computed in the test, so a change to how the hash is taken fails here instead
// of agreeing with itself.
const (
	testCard     = `{"name":"Orderbook Analyst","description":"Analyzes order books and funding rates.","skills":[{"name":"depth","description":"Order book depth analysis."}]}`
	testCardHash = "WflqJrx0K07EBJ2zNfRBdnR9QJjyATd2dWzY12niCj8="
	zeroHash     = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
)

func TestCardTrust(t *testing.T) {
	card := testCard
	other := `{"name":"Someone Else"}`

	for _, tc := range []struct {
		name string
		hit  Hit
		want CardTrust
	}{
		{
			name: "verified when the served bytes hash to the committed value",
			hit:  Hit{Card: &card, CapabilityHash: testCardHash, HealthStatus: "healthy"},
			want: CardVerified,
		},
		{
			name: "verified without a health status (older market build)",
			hit:  Hit{Card: &card, CapabilityHash: testCardHash},
			want: CardVerified,
		},
		{
			name: "mismatch when the card is not the committed one",
			hit:  Hit{Card: &other, CapabilityHash: testCardHash, HealthStatus: "healthy"},
			want: CardMismatch,
		},
		{
			// The market keeps the last card that verified and only flips the
			// status, so matching bytes plus an unhealthy status means this card
			// is superseded, not current.
			name: "mismatch when the market's latest fetch failed",
			hit:  Hit{Card: &card, CapabilityHash: testCardHash, HealthStatus: "hash_mismatch"},
			want: CardMismatch,
		},
		{
			name: "unverified when the owner committed no hash",
			hit:  Hit{Card: &card, CapabilityHash: zeroHash},
			want: CardUnverified,
		},
		{
			name: "unverified when no card was served",
			hit:  Hit{CapabilityHash: testCardHash},
			want: CardUnverified,
		},
		{
			name: "unverified when the committed hash is unreadable",
			hit:  Hit{Card: &card, CapabilityHash: "not-base64!!"},
			want: CardUnverified,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.hit.CardTrust())
		})
	}
}

// The chain commits to the bytes as served. Re-encoding a card — even into
// identical JSON semantics — must not verify, or the whole check is decorative.
func TestCardTrustRejectsReencodedCard(t *testing.T) {
	reencoded := `{"description":"Analyzes order books and funding rates.","name":"Orderbook Analyst","skills":[{"description":"Order book depth analysis.","name":"depth"}]}`
	hit := Hit{Card: &reencoded, CapabilityHash: testCardHash, HealthStatus: "healthy"}
	require.Equal(t, CardMismatch, hit.CardTrust())
}

func TestParseCard(t *testing.T) {
	card, ok := ParseCard(testCard)
	require.True(t, ok)
	require.Equal(t, "Orderbook Analyst", card.Name)
	require.Equal(t, "Analyzes order books and funding rates.", card.Description)
	require.Len(t, card.Skills, 1)
	require.Equal(t, "depth", card.Skills[0].Name)
	require.Equal(t, "Order book depth analysis.", card.Skills[0].Description)
}

func TestParseCardRejectsUnusableCards(t *testing.T) {
	_, ok := ParseCard("not json at all")
	require.False(t, ok, "a non-JSON card has no projection")

	_, ok = ParseCard(`{"version":"1.0","supportedInterfaces":["jsonrpc"]}`)
	require.False(t, ok, "a card with no human-language text has no projection")
}

// A card is third-party text that reaches the context a page of agents at a
// time, so no single agent may spend the page's budget on itself.
func TestParseCardBoundsThirdPartyText(t *testing.T) {
	var skills []string
	for i := 0; i < 20; i++ {
		skills = append(skills, `{"name":"s","description":"d"}`)
	}
	raw := `{"name":"` + strings.Repeat("n", 500) +
		`","description":"` + strings.Repeat("d", 900) +
		`","skills":[` + strings.Join(skills, ",") + `]}`

	card, ok := ParseCard(raw)
	require.True(t, ok)
	require.LessOrEqual(t, len(card.Name), cardNameMax+len("…"))
	require.LessOrEqual(t, len(card.Description), cardDescMax+len("…"))
	require.Len(t, card.Skills, cardSkillsMax)
}

// An agent with many verbose skills must not spend the whole page's budget on
// itself, but must still come back with something to judge it by.
func TestParseCardBoundsTheSkillListAsAWhole(t *testing.T) {
	var skills []string
	for i := 0; i < cardSkillsMax; i++ {
		skills = append(skills, `{"name":"skill","description":"`+strings.Repeat("d", cardSkillDescMax)+`"}`)
	}
	card, ok := ParseCard(`{"name":"Verbose","skills":[` + strings.Join(skills, ",") + `]}`)
	require.True(t, ok)
	require.NotEmpty(t, card.Skills, "an agent is never reduced to a name alone")
	require.Less(t, len(card.Skills), cardSkillsMax, "the budget must stop a verbose agent short")
}

// An svpchain agent appends "Tools: a, b, c." to each skill description, built
// from the registry that its list_tools serves. That tail starts well past 140
// characters on a real EVM agent, and it is the part that makes one agent
// distinguishable from another, so the projection must not cut it off.
func TestParseCardKeepsSvpchainToolTail(t *testing.T) {
	description := "EVM transaction broadcast and status. Additional private MCP tools are " +
		"synchronized at startup and published here only when the private service is " +
		"reachable. Tools: broadcast_evm_tx, build_erc20_approve, build_erc20_transfer, " +
		"build_swap, build_token_approval, evm_tx_status, get_balance, " +
		"get_transfer_out_cap, list_evm_assets, quote_swap, set_transfer_out_cap."
	raw, err := json.Marshal(map[string]any{
		"name":   "svpchain-evm-agent",
		"skills": []any{map[string]any{"name": "SVP-Chain EVM DeFi", "description": description}},
	})
	require.NoError(t, err)

	card, ok := ParseCard(string(raw))
	require.True(t, ok)
	require.Len(t, card.Skills, 1)
	require.Contains(t, card.Skills[0].Description, "Tools:")
	require.Contains(t, card.Skills[0].Description, "build_swap")
}

// A card is third-party text and routinely not ASCII. The bound is in bytes, so
// truncation must walk back to a rune boundary — cutting a multi-byte character
// in half yields invalid UTF-8, which json quietly rewrites to U+FFFD instead of
// rejecting, so nothing else would catch it.
func TestClampTextTruncatesOnRuneBoundaries(t *testing.T) {
	// The offsets matter: cardDescMax is divisible by 3, so a pure 3-byte-rune
	// string happens to cut cleanly and would hide the bug on its own.
	for _, prefix := range []string{"", "a", "ab"} {
		got := clampText(prefix+strings.Repeat("链", 200), cardDescMax)
		require.True(t, utf8.ValidString(got), "prefix %q produced invalid UTF-8: %q", prefix, got)
		require.LessOrEqual(t, len(got), cardDescMax+len("…"))
	}
	for _, prefix := range []string{"", "a", "ab", "abc"} {
		got := clampText(prefix+strings.Repeat("🚀", 200), cardDescMax)
		require.True(t, utf8.ValidString(got), "prefix %q produced invalid UTF-8: %q", prefix, got)
	}
}

// Newlines are collapsed so one agent's card cannot reformat the result the
// model reads around it.
func TestParseCardCollapsesWhitespace(t *testing.T) {
	card, ok := ParseCard(`{"name":"A","description":"line one\n\nline two\t tail"}`)
	require.True(t, ok)
	require.Equal(t, "line one line two tail", card.Description)
}

func TestGetDecodesCardFields(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"agent_id":        "did:svp:evm",
		"endpoint":        "http://agent",
		"capability_hash": testCardHash,
		"health_status":   "healthy",
		"card":            testCard,
	})
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	hit, err := New(server.URL).Get(context.Background(), "did:svp:evm")
	require.NoError(t, err)
	require.NotNil(t, hit.Card)
	// Verbatim: the bytes the hash is taken over must survive the round trip.
	require.Equal(t, testCard, *hit.Card)
	require.Equal(t, CardVerified, hit.CardTrust())
}
