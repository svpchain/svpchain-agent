// Package a2amcp reaches an svpchain agent's MCP tool surface over A2A.
//
// A remote agent built on svpchain-evm-agent's internal/toolbridge exposes the
// same MCP tool handlers the remote MCP server does — same names, same
// argument schemas — wrapped in a JSON envelope carried as A2A message text:
//
//	→ {"skill":"svpchain-evm","tool":"build_swap","args":{…}}
//	← {"skill":"svpchain-evm","tool":"build_swap","ok":true,"result":{…}}
//
// That sameness is the whole point, and the reason this is a transport rather
// than a new tool surface. A tool reached this way keeps its MCP name, so the
// whitelist gate (package guard) reads `recipient` / `to` / `spender` out of
// its arguments exactly as it does for a remote build_*, and the write-path
// graph (package writepath) classifies build_* → sign_* → broadcast_* without
// knowing which transport carried it. Nothing here may translate, rename, or
// reshape a tool: the moment a name diverges, both of those layers go blind.
//
// What IS different from the remote MCP is provenance. This endpoint was named
// by a search index, not by the user's configuration, so it must never be able
// to displace something trusted. This package refuses to carry a sign_* tool
// at all, and the caller applies the full precedence rule (local signer >
// remote MCP > A2A) when merging these tools into a run's tool list.
package a2amcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	svpa2a "github.com/svpchain/svpchain-agent/internal/a2a"
	"github.com/svpchain/svpchain-agent/internal/agent/llm"
)

// The self-description surface every caller-signed svpchain agent serves.
const (
	metaSkill     = "svpchain-meta"
	listToolsTool = "list_tools"
)

// request is the envelope the remote executor decodes (its a2aserver.Request).
type request struct {
	Skill  string `json:"skill"`
	Tool   string `json:"tool"`
	Args   any    `json:"args,omitempty"`
	Bearer string `json:"bearer,omitempty"`
}

// reply is its a2aserver.Response. A refused or failed operation arrives as a
// completed task with OK=false, not as a transport error — so a tool refusal
// reaches the model as a tool error, the same as one from the remote MCP.
type reply struct {
	Skill  string          `json:"skill"`
	Tool   string          `json:"tool"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type toolDescriptor struct {
	Skill       string          `json:"skill"`
	Tool        string          `json:"tool"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type listToolsResult struct {
	Tools []toolDescriptor `json:"tools"`
}

// Client is one connected remote agent.
type Client struct {
	url string

	mu sync.Mutex
	// contextID pins every call to one A2A context. The remote binds a minted
	// bearer to it, so losing it silently de-authenticates the conversation.
	contextID   string
	bearer      string
	bearerUntil time.Time
	skills      map[string]string // tool → the skill it dispatches under
	tools       []llm.Tool
}

func New(agentURL string) *Client {
	return &Client{url: strings.TrimSpace(agentURL), skills: map[string]string{}}
}

func (c *Client) URL() string { return c.url }

// Connect asks the agent what it serves and records the result. It is the only
// thing that populates the tool list, so an agent that does not answer
// list_tools contributes nothing rather than a guess.
func (c *Client) Connect(ctx context.Context) error {
	if c.url == "" {
		return fmt.Errorf("agent_url is required")
	}
	raw, err := c.call(ctx, metaSkill, listToolsTool, map[string]any{})
	if err != nil {
		return fmt.Errorf(
			"agent %s does not serve %s/%s, so its tools cannot be discovered: %w",
			c.url, metaSkill, listToolsTool, err)
	}
	var listed listToolsResult
	if err := json.Unmarshal(raw, &listed); err != nil {
		return fmt.Errorf("decode %s from %s: %w", listToolsTool, c.url, err)
	}

	skills := map[string]string{}
	tools := make([]llm.Tool, 0, len(listed.Tools))
	for _, d := range listed.Tools {
		name := strings.TrimSpace(d.Tool)
		if name == "" || strings.TrimSpace(d.Skill) == "" {
			continue
		}
		// A remote agent supplying a signing tool is either broken or hostile;
		// either way the local signer owns those names.
		if strings.HasPrefix(name, "sign_") {
			continue
		}
		// svpchain-meta is this transport's own control channel. Connect calls
		// list_tools directly; re-advertising it would offer plumbing as if it
		// were a capability, and its discovery envelope is not a tool result.
		if strings.TrimSpace(d.Skill) == metaSkill {
			continue
		}
		params := map[string]any{"type": "object"}
		if len(d.InputSchema) > 0 {
			var decoded map[string]any
			if err := json.Unmarshal(d.InputSchema, &decoded); err == nil && len(decoded) > 0 {
				params = decoded
			}
		}
		skills[name] = d.Skill
		tools = append(tools, llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name: name,
				Description: fmt.Sprintf(
					"%s, served by the remote agent at %s under skill %s. Same tool and arguments as the "+
						"svpchain MCP tool of this name; results build → sign locally → broadcast as usual.",
					name, c.url, d.Skill),
				Parameters: params,
			},
		})
	}
	if len(tools) == 0 {
		return fmt.Errorf("agent %s listed no usable tools", c.url)
	}

	c.mu.Lock()
	c.skills = skills
	c.tools = tools
	c.mu.Unlock()
	return nil
}

// Tools returns what this agent serves, as discovered by Connect.
func (c *Client) Tools() []llm.Tool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]llm.Tool(nil), c.tools...)
}

// Handles reports whether this agent advertised name.
func (c *Client) Handles(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.skills[strings.TrimSpace(name)]
	return ok
}

// CallTool invokes one of the agent's tools and returns its result as JSON —
// the same shape the remote MCP returns for the same tool name.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	name = strings.TrimSpace(name)
	c.mu.Lock()
	skill, ok := c.skills[name]
	c.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("agent %s does not serve %q", c.url, name)
	}
	if args == nil {
		args = map[string]any{}
	}
	raw, err := c.call(ctx, skill, name, args)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// call sends one envelope and returns the result payload. A non-OK reply
// becomes an error carrying the agent's own message, so the model sees the
// same refusal text the MCP transport would have produced.
func (c *Client) call(ctx context.Context, skill, tool string, args any) (json.RawMessage, error) {
	c.mu.Lock()
	req := request{Skill: skill, Tool: tool, Args: args, Bearer: c.bearer}
	contextID := c.contextID
	c.mu.Unlock()

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode %s request: %w", tool, err)
	}
	res, err := sendText(ctx, c.url, contextID, string(body))
	if err != nil {
		return nil, err
	}
	// Adopt the context the agent assigned, so a bearer minted later in this
	// conversation stays bound to it.
	if id := strings.TrimSpace(res.ContextID); id != "" {
		c.mu.Lock()
		if c.contextID == "" {
			c.contextID = id
		}
		c.mu.Unlock()
	}

	text := strings.TrimSpace(res.Response)
	if text == "" {
		return nil, fmt.Errorf("agent %s returned an empty reply for %s", c.url, tool)
	}
	var out reply
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		// The executor answers a malformed request with bare "error: …" text.
		return nil, fmt.Errorf("agent %s did not answer %s in its envelope: %s", c.url, tool, truncate(text, 400))
	}
	if !out.OK {
		if msg := strings.TrimSpace(out.Error); msg != "" {
			return nil, fmt.Errorf("%s", msg)
		}
		return nil, fmt.Errorf("agent %s refused %s without a reason", c.url, tool)
	}
	if len(out.Result) == 0 {
		return json.RawMessage("{}"), nil
	}
	return out.Result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// sendText is overridden in tests.
var sendText = func(ctx context.Context, agentURL, contextID, text string) (svpa2a.SendResult, error) {
	return svpa2a.SendTextIn(ctx, agentURL, contextID, text)
}

type authChallengeOut struct {
	Challenge string `json:"challenge"`
	Nonce     string `json:"nonce"`
	ExpiresAt int64  `json:"expires_at"`
}

type authVerifyOut struct {
	BearerToken string `json:"bearer_token"`
	Owner       string `json:"owner"`
	ExpiresAt   int64  `json:"expires_at"`
}

// BearerValid reports whether this client holds an unexpired bearer.
func (c *Client) BearerValid() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bearer != "" && time.Now().Before(c.bearerUntil)
}

// EnsureAuth runs the agent's handshake: auth_challenge → signChallenge →
// auth_verify. It is the same exchange the remote MCP uses, and deliberately
// so: the challenge the agent issues carries the svpchain-mcp-auth-v1 prefix
// and this chain's id, which is exactly what the local sign_challenge tool
// requires — a challenge shaped any other way is refused by the signer, not
// worked around here.
//
// signChallenge must be the LOCAL signer. Nothing in this exchange sends a key
// anywhere: the agent gets a signature over its own challenge, and mints a
// bearer scoped to this conversation.
func (c *Client) EnsureAuth(ctx context.Context, owner string, signChallenge func(challenge string) (string, error)) error {
	if c.BearerValid() {
		return nil
	}
	if strings.TrimSpace(owner) == "" {
		return fmt.Errorf("owner address is required to authenticate with %s", c.url)
	}
	if signChallenge == nil {
		return fmt.Errorf("no local signer available to answer %s's auth challenge", c.url)
	}
	if !c.Handles("auth_challenge") || !c.Handles("auth_verify") {
		return fmt.Errorf("agent %s serves no auth handshake", c.url)
	}

	chRaw, err := c.CallTool(ctx, "auth_challenge", map[string]any{"owner": owner})
	if err != nil {
		return fmt.Errorf("auth_challenge: %w", err)
	}
	var ch authChallengeOut
	if err := json.Unmarshal([]byte(chRaw), &ch); err != nil {
		return fmt.Errorf("parse auth_challenge: %w", err)
	}
	sig, err := signChallenge(ch.Challenge)
	if err != nil {
		return fmt.Errorf("sign_challenge: %w", err)
	}
	verifyRaw, err := c.CallTool(ctx, "auth_verify", map[string]any{"nonce": ch.Nonce, "signature": sig})
	if err != nil {
		return fmt.Errorf("auth_verify: %w", err)
	}
	var vr authVerifyOut
	if err := json.Unmarshal([]byte(verifyRaw), &vr); err != nil {
		return fmt.Errorf("parse auth_verify: %w", err)
	}
	if strings.TrimSpace(vr.BearerToken) == "" {
		return fmt.Errorf("agent %s returned no bearer token", c.url)
	}

	c.mu.Lock()
	c.bearer = vr.BearerToken
	if vr.ExpiresAt > 0 {
		c.bearerUntil = time.Unix(vr.ExpiresAt, 0)
	} else {
		c.bearerUntil = time.Now().Add(24 * time.Hour)
	}
	c.mu.Unlock()
	return nil
}
