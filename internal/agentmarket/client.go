// Package agentmarket is a read-only client for the SVP Agent Market service's
// semantic search API — the assistant's only way to find remote agents.
//
// # Trust
//
// The market service is a remote party, separate from the remote MCP server.
// It indexes the chain's x/agent registry, but nothing here can prove that:
// this client reports what the service returns, including each agent's A2A
// endpoint. Nothing verifies that endpoint against the chain.
//
// What that does and does not put at risk: a search result decides where an
// `a2a_send_message` goes, so a hostile or stale index can point the assistant
// at an endpoint of its choosing and see whatever is in that message. It cannot
// move the user's funds — A2A messages carry no credential, and every on-chain
// write still goes through remote build → local sign (user-confirmed, whitelist
// checked) → remote broadcast.
package agentmarket

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Client reads the market service's search API.
type Client struct {
	base string
	http *http.Client
}

// DefaultURL is used when no Agent Market URL is configured, so agent search
// works out of the box. Mirrors how the remote MCP client defaults its own
// endpoint; clearing the setting restores this default rather than disabling
// search.
const DefaultURL = "https://agent-market-devnet.svpstars.com"

// New returns a client for baseURL, falling back to DefaultURL when empty.
func New(baseURL string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultURL
	}
	return &Client{base: baseURL, http: &http.Client{Timeout: 20 * time.Second}}
}

// BaseURL reports the configured endpoint, for display in tool results.
func (c *Client) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.base
}

// Coin is a chain coin with the amount kept as its decimal string.
type Coin struct {
	Denom  string `json:"denom,omitempty"`
	Amount string `json:"amount,omitempty"`
}

// Pricing is an agent's advertised price in the network AgentSettlement
// payment token's smallest unit. The token is selected by the network
// settlement deployment, not by an individual agent record.
type Pricing struct {
	Amount string `json:"amount,omitempty"`
	Unit   string `json:"unit,omitempty"`
}

// Hit is one search result: an agent as the market service describes it, plus
// how well it matched. Every field except Similarity is the service's claim
// about an on-chain record — see the package comment.
type Hit struct {
	AgentID      string   `json:"agent_id"`
	Owner        string   `json:"owner,omitempty"`
	Endpoint     string   `json:"endpoint,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Pricing      Pricing  `json:"pricing,omitzero"`
	Bond         Coin     `json:"bond,omitzero"`
	Status       string   `json:"status,omitempty"`
	Metadata     string   `json:"metadata,omitempty"`
	Similarity   float64  `json:"similarity"`

	// CapabilityHash is the SHA-256 (base64) the owner committed on chain, over
	// the Agent Card bytes exactly as its endpoint serves them. All zeroes means
	// the agent registered without committing to a card at all.
	CapabilityHash string `json:"capability_hash,omitempty"`
	// HealthStatus and HealthError are the market's own verdict on the card it
	// last fetched ("healthy", "hash_mismatch", "unreachable", ...). Carried for
	// diagnostics; CardTrust re-derives the verdict here rather than trusting it.
	HealthStatus string `json:"health_status,omitempty"`
	HealthError  string `json:"health_error,omitempty"`
	// Card is the A2A Agent Card as fetched from the agent's own endpoint — a
	// JSON *string*, never a decoded object, because CapabilityHash is taken
	// over these exact bytes and re-encoding them (key order, whitespace) would
	// break verification. Nil when the market has never fetched one, or when the
	// service is too old to serve the field.
	//
	// Distinct from the chain's registration record: this is what the agent
	// says about itself, not what its owner registered.
	Card *string `json:"card,omitempty"`
}

// CardTrust is the verdict on a served Agent Card, checked against the hash its
// owner committed on chain.
type CardTrust string

const (
	// CardVerified means the served card's bytes hash to CapabilityHash, so it
	// is the card the owner registered and bonded against.
	CardVerified CardTrust = "verified"
	// CardUnverified means there is nothing to check: no card was served, or the
	// owner committed no hash. Not a failure — an absence of evidence.
	CardUnverified CardTrust = "unverified"
	// CardMismatch means a card was served but it is not the committed one. The
	// market keeps the last card that DID verify and only flips its health
	// status, so such a card is typically a real but superseded self-description
	// — which makes it more misleading than no card at all.
	CardMismatch CardTrust = "mismatch"
)

// CardTrust checks the served card against the chain's committed hash.
//
// Deliberately recomputed here instead of read out of HealthStatus: the market
// is the party making the claim, so its own verdict is not evidence for it. The
// hash being checked against still comes from the market — nothing in this repo
// reads the chain — so this catches a stale or inconsistent index, not a market
// that lies about card and hash together.
func (h Hit) CardTrust() CardTrust {
	if h.Card == nil || strings.TrimSpace(*h.Card) == "" {
		return CardUnverified
	}
	want, err := base64.StdEncoding.DecodeString(strings.TrimSpace(h.CapabilityHash))
	if err != nil || len(want) != sha256.Size || isZeroHash(want) {
		return CardUnverified
	}
	sum := sha256.Sum256([]byte(*h.Card))
	if !bytes.Equal(sum[:], want) {
		return CardMismatch
	}
	// The bytes verify, but the market says its most recent fetch did not: this
	// is the last good card, not what the endpoint serves now.
	if status := strings.TrimSpace(h.HealthStatus); status != "" && status != "healthy" {
		return CardMismatch
	}
	return CardVerified
}

func isZeroHash(sum []byte) bool {
	for _, b := range sum {
		if b != 0 {
			return false
		}
	}
	return true
}

// Bounds on the projection below. This is third-party text that reaches the
// assistant's context a whole search page at a time, so no single agent may
// spend the page's budget on itself.
//
// cardSkillDescMax is generous on purpose. An svpchain agent builds each skill
// description from its live tool registry and appends "Tools: a, b, c." — on a
// real EVM agent that tail starts 158 characters in, so a tight per-skill bound
// truncates away the one part that helps choose between agents. The skill list
// as a whole is capped by cardSkillsBudget instead, which bounds an agent with
// many skills without mutilating an agent with a few informative ones.
const (
	cardNameMax      = 80
	cardDescMax      = 240
	cardSkillsMax    = 6
	cardSkillDescMax = 400
	cardSkillsBudget = 560
)

// CardSkill is one skill as an agent's own card advertises it. A skill is not a
// tool: what an attached agent can actually be called with comes from its
// list_tools reply, never from here.
type CardSkill struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// Card is the human-language part of an A2A Agent Card — what an agent says it
// is and what it says it can do. Capability tags alone rarely make a search
// result decidable; this is the part that does.
//
// It is a bounded projection rather than the card itself: transport detail
// (interfaces, mime types, provider, version) helps no one choose an agent, and
// the text that remains is truncated.
type Card struct {
	Name        string      `json:"name,omitempty"`
	Description string      `json:"description,omitempty"`
	Skills      []CardSkill `json:"skills,omitempty"`
}

// rawCard is the subset of the A2A card shape this projection reads. It mirrors
// what the market embeds for ranking, so what an agent is searchable by and what
// it is shown as stay the same text.
type rawCard struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Skills      []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"skills"`
}

// ParseCard projects the human-language fields out of a raw Agent Card.
//
// Reports false when the card is not JSON, or parses but carries no usable
// text. Both are ordinary outcomes rather than errors: a card is third-party
// content, and the market stores whatever verified against the chain hash,
// including shapes this does not recognize.
func ParseCard(raw string) (Card, bool) {
	var parsed rawCard
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return Card{}, false
	}
	card := Card{
		Name:        clampText(parsed.Name, cardNameMax),
		Description: clampText(parsed.Description, cardDescMax),
	}
	spent := 0
	for _, skill := range parsed.Skills {
		name := clampText(skill.Name, cardNameMax)
		description := clampText(skill.Description, cardSkillDescMax)
		if name == "" && description == "" {
			continue
		}
		if len(card.Skills) == cardSkillsMax {
			break
		}
		// Always keep the first skill, so an agent is never reduced to a name
		// alone; past that, stop once the list has spent its budget.
		if spent+len(name)+len(description) > cardSkillsBudget && len(card.Skills) > 0 {
			break
		}
		spent += len(name) + len(description)
		card.Skills = append(card.Skills, CardSkill{Name: name, Description: description})
	}
	if card.Name == "" && card.Description == "" && len(card.Skills) == 0 {
		return Card{}, false
	}
	return card, true
}

// clampText trims a card string to a bounded, single-line form. Newlines are
// collapsed so one agent's card cannot reformat the result the model reads.
//
// The bound is in bytes, so the cut is walked back to a rune boundary: an agent
// card is third-party text and routinely not ASCII, and slicing a multi-byte
// character in half would put invalid UTF-8 into the tool result — which json
// silently rewrites to U+FFFD rather than rejecting.
func clampText(text string, max int) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if len(text) <= max {
		return text
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return strings.TrimSpace(text[:cut]) + "…"
}

// Query narrows a search.
type Query struct {
	Text       string
	Capability string
	Limit      int
}

type searchResponse struct {
	Agents []Hit  `json:"agents"`
	Error  string `json:"error"`
}

// ListQuery narrows a paginated listing of active Agent records.
type ListQuery struct {
	Capability string
	Limit      int
	Cursor     int
}

// Page is one page from the Agent Market listing API.
type Page struct {
	Agents     []Hit `json:"agents"`
	Limit      int   `json:"limit"`
	Cursor     int   `json:"cursor"`
	NextCursor int   `json:"next_cursor"`
}

// Get returns the current on-chain-derived record for one agent.
func (c *Client) Get(ctx context.Context, agentID string) (Hit, error) {
	if c == nil {
		return Hit{}, fmt.Errorf("agent market search is not configured")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return Hit{}, fmt.Errorf("agent id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/agents/"+url.PathEscape(agentID), nil)
	if err != nil {
		return Hit{}, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return Hit{}, fmt.Errorf("agent market unreachable: %w", err)
	}
	defer response.Body.Close()
	var hit Hit
	if err := json.NewDecoder(response.Body).Decode(&hit); err != nil && response.StatusCode == http.StatusOK {
		return Hit{}, fmt.Errorf("decode agent market response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return Hit{}, fmt.Errorf("agent market lookup failed (HTTP %d)", response.StatusCode)
	}
	hits := usableHits([]Hit{hit})
	if len(hits) != 1 {
		return Hit{}, fmt.Errorf("agent market lookup returned an invalid agent record")
	}
	return hits[0], nil
}

type listResponse struct {
	Page
	Error string `json:"error"`
}

// List returns a page of active Agent records without semantic ranking.
func (c *Client) List(ctx context.Context, q ListQuery) (Page, error) {
	if c == nil {
		return Page{}, fmt.Errorf("agent market search is not configured")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}
	values := url.Values{}
	values.Set("limit", strconv.Itoa(limit))
	values.Set("cursor", strconv.Itoa(max(q.Cursor, 0)))
	values.Set("status", "AGENT_STATUS_ACTIVE")
	if cap := strings.TrimSpace(q.Capability); cap != "" {
		values.Set("capability", cap)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/agents?"+values.Encode(), nil)
	if err != nil {
		return Page{}, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return Page{}, fmt.Errorf("agent market unreachable: %w", err)
	}
	defer response.Body.Close()

	var decoded listResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil && response.StatusCode == http.StatusOK {
		return Page{}, fmt.Errorf("decode agent market response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		if decoded.Error != "" {
			return Page{}, fmt.Errorf("agent market list failed (HTTP %d): %s", response.StatusCode, decoded.Error)
		}
		return Page{}, fmt.Errorf("agent market list failed (HTTP %d)", response.StatusCode)
	}
	decoded.Agents = usableHits(decoded.Agents)
	return decoded.Page, nil
}

// FindActiveByEndpoint resolves an active market record by its advertised A2A
// endpoint. The market currently exposes no endpoint index, so this walks its
// bounded paginated listing. A caller must not fall back to a free execution
// when this lookup fails: that would let a market agent evade settlement on a
// later chat turn, after the original search result has left process memory.
func (c *Client) FindActiveByEndpoint(ctx context.Context, endpoint string) (Hit, bool, error) {
	if c == nil {
		return Hit{}, false, fmt.Errorf("agent market search is not configured")
	}
	want := normalizeEndpoint(endpoint)
	if want == "" {
		return Hit{}, false, fmt.Errorf("agent endpoint is required")
	}

	const (
		pageSize = 200
		maxPages = 50
	)
	cursor := 0
	for pageNumber := 0; pageNumber < maxPages; pageNumber++ {
		page, err := c.List(ctx, ListQuery{Limit: pageSize, Cursor: cursor})
		if err != nil {
			return Hit{}, false, err
		}
		for _, hit := range page.Agents {
			if normalizeEndpoint(hit.Endpoint) == want {
				return hit, true, nil
			}
		}
		if len(page.Agents) == 0 || page.NextCursor <= cursor {
			return Hit{}, false, nil
		}
		cursor = page.NextCursor
	}
	return Hit{}, false, fmt.Errorf("agent market endpoint lookup exceeded %d pages", maxPages)
}

// Search returns agents ranked by semantic similarity to q.Text.
func (c *Client) Search(ctx context.Context, q Query) ([]Hit, error) {
	if c == nil {
		return nil, fmt.Errorf("agent market search is not configured")
	}
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return nil, fmt.Errorf("search text is required")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}
	values := url.Values{}
	values.Set("q", text)
	values.Set("limit", strconv.Itoa(limit))
	// Only ACTIVE agents can be acted on, so never spend ranking slots on ones
	// that cannot.
	values.Set("status", "AGENT_STATUS_ACTIVE")
	if cap := strings.TrimSpace(q.Capability); cap != "" {
		values.Set("capability", cap)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/agents/search?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent market unreachable: %w", err)
	}
	defer response.Body.Close()

	var decoded searchResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil && response.StatusCode == http.StatusOK {
		return nil, fmt.Errorf("decode agent market response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		if decoded.Error != "" {
			return nil, fmt.Errorf("agent market search failed (HTTP %d): %s", response.StatusCode, decoded.Error)
		}
		return nil, fmt.Errorf("agent market search failed (HTTP %d)", response.StatusCode)
	}
	return usableHits(decoded.Agents), nil
}

func usableHits(agents []Hit) []Hit {
	hits := make([]Hit, 0, len(agents))
	for _, a := range agents {
		a.AgentID = strings.TrimSpace(a.AgentID)
		if a.AgentID == "" {
			continue
		}
		a.Endpoint = normalizeEndpoint(a.Endpoint)
		hits = append(hits, a)
	}
	return hits
}

// normalizeEndpoint makes an endpoint emitted by the market and the same
// endpoint sent back to a2a_connect_agent compare consistently. A trailing
// slash is not meaningful for the A2A base endpoint used here.
func normalizeEndpoint(endpoint string) string {
	return strings.TrimRight(strings.TrimSpace(endpoint), "/")
}
