package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/svpchain/svpchain-agent/internal/agent/result"
)

const defaultRemoteURL = "https://indexer.svpchain.com/mcp"

type bearerRoundTripper struct {
	base   http.RoundTripper
	bearer func() string
}

func (t *bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if token := t.bearer(); token != "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return base.RoundTrip(req)
}

// Client talks to the svpchain remote MCP server over Streamable HTTP.
type Client struct {
	url     string
	client  *mcp.Client
	session *mcp.ClientSession

	mu          sync.Mutex
	bearer      string
	bearerUntil time.Time

	forceConnected bool // tests only: treat client as connected without a live session
}

// NewClient creates a client for endpoint (empty → production default).
func NewClient(endpoint string) *Client {
	if endpoint == "" {
		endpoint = defaultRemoteURL
	}
	return &Client{url: endpoint}
}

// Connect opens the MCP session.
func (r *Client) Connect(ctx context.Context) error {
	r.mu.Lock()
	if r.session != nil || r.forceConnected {
		r.mu.Unlock()
		return nil
	}
	rt := &bearerRoundTripper{bearer: r.currentBearer}
	httpClient := &http.Client{
		Transport: rt,
		Timeout:   90 * time.Second,
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint:   r.url,
		HTTPClient: httpClient,
	}
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "svpchain-gui",
		Version: "v0.1.0",
	}, nil)
	r.client = client
	// Release the lock before the network handshake: r.client.Connect issues an
	// HTTP request through bearerRoundTripper, which re-acquires r.mu via
	// currentBearer. Holding the lock here would self-deadlock (sync.Mutex is
	// not reentrant) and hang the connect forever.
	r.mu.Unlock()

	sess, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("connect remote mcp: %w", err)
	}
	r.mu.Lock()
	r.session = sess
	r.mu.Unlock()
	return nil
}

// IsConnected reports whether the MCP session is open.
func (r *Client) IsConnected() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.session != nil || r.forceConnected
}

// BearerValid reports whether the cached bearer token is still usable.
func (r *Client) BearerValid() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bearer != "" && time.Now().Before(r.bearerUntil.Add(-time.Minute))
}

// currentBearer returns the live bearer token if still valid, else "".
func (r *Client) currentBearer() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bearer != "" && time.Now().Before(r.bearerUntil) {
		return r.bearer
	}
	return ""
}

// Close ends the MCP session.
func (r *Client) Close() error {
	r.mu.Lock()
	sess := r.session
	r.session = nil
	r.client = nil
	r.forceConnected = false
	r.mu.Unlock()
	if sess == nil {
		return nil
	}
	// Close outside the lock: session.Close issues an HTTP request through
	// bearerRoundTripper, which re-acquires r.mu via currentBearer. Holding the
	// lock here would self-deadlock and hang Run's deferred Close forever.
	return sess.Close()
}

// ListTools returns remote tool definitions for the LLM.
func (r *Client) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	r.mu.Lock()
	sess := r.session
	r.mu.Unlock()
	if sess == nil {
		return nil, fmt.Errorf("remote mcp not connected")
	}
	res, err := sess.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	return res.Tools, nil
}

// CallTool invokes a remote tool.
func (r *Client) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	r.mu.Lock()
	sess := r.session
	r.mu.Unlock()
	if sess == nil {
		return "", fmt.Errorf("remote mcp not connected")
	}
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return "", err
	}
	text, err := result.ToolText(res)
	if err != nil {
		return "", err
	}
	if res.IsError {
		return text, fmt.Errorf("%s", text)
	}
	return text, nil
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

// EnsureAuth runs auth_challenge → signChallenge → auth_verify when needed.
func (r *Client) EnsureAuth(ctx context.Context, owner string, signChallenge func(challenge string) (signatureB64 string, err error)) error {
	if r.BearerValid() {
		return nil
	}
	chRes, err := r.CallTool(ctx, "auth_challenge", map[string]any{"owner": owner})
	if err != nil {
		return fmt.Errorf("auth_challenge: %w", err)
	}
	var ch authChallengeOut
	if err := json.Unmarshal([]byte(chRes), &ch); err != nil {
		return fmt.Errorf("parse auth_challenge: %w", err)
	}
	sig, err := signChallenge(ch.Challenge)
	if err != nil {
		return fmt.Errorf("sign_challenge: %w", err)
	}
	verifyRes, err := r.CallTool(ctx, "auth_verify", map[string]any{
		"nonce":     ch.Nonce,
		"signature": sig,
	})
	if err != nil {
		return fmt.Errorf("auth_verify: %w", err)
	}
	var vr authVerifyOut
	if err := json.Unmarshal([]byte(verifyRes), &vr); err != nil {
		return fmt.Errorf("parse auth_verify: %w", err)
	}
	r.mu.Lock()
	r.bearer = vr.BearerToken
	if vr.ExpiresAt > 0 {
		r.bearerUntil = time.Unix(vr.ExpiresAt, 0)
	} else {
		r.bearerUntil = time.Now().Add(24 * time.Hour)
	}
	r.mu.Unlock()
	return nil
}
