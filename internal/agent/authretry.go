package agent

import (
	"context"
	"strings"

	"github.com/svpchain/svpchain-agent/internal/agent/a2amcp"
	remotemcp "github.com/svpchain/svpchain-agent/internal/agent/remote"
)

// authRequiredMarker opens the "authenticate first" reply every svpchain MCP
// service returns for a tenant-scoped tool called without a bearer. The text is
// identical across the dex, defi, evm, lending and perps services (their
// tools.ErrNoTenant), which is what makes matching on it viable.
//
// It usually arrives as a SUCCESSFUL result rather than an error: the services
// deliberately return a soft result, because an MCP error aborts agent loops
// that break out on tool failure. So this must be checked against result text,
// not only against errors.
const authRequiredMarker = "authentication required: this tool needs a bearer token"

// isAuthRequired reports whether a tool call came back asking for the handshake,
// in either of the two forms a service may use.
func isAuthRequired(text string, err error) bool {
	if strings.Contains(text, authRequiredMarker) {
		return true
	}
	return err != nil && strings.Contains(err.Error(), authRequiredMarker)
}

// isHandshakeTool reports whether name is part of the handshake itself. Retrying
// these on an auth_required answer would recurse: the handshake cannot
// authenticate itself.
func isHandshakeTool(name string) bool {
	switch strings.TrimSpace(name) {
	case "auth_challenge", "auth_verify":
		return true
	}
	return false
}

// authRetrier is the part of a tool transport that can re-run the handshake.
// *remote.Client and *a2amcp.Client both implement it.
type authRetrier interface {
	InvalidateBearer()
	EnsureAuth(ctx context.Context, owner string, signChallenge func(challenge string) (string, error)) error
}

// Pinned at compile time: attached.retrier reaches the A2A client through a type
// assertion, so a drifting signature there would silently stop re-authenticating
// instead of failing to build.
var (
	_ authRetrier = (*remotemcp.Client)(nil)
	_ authRetrier = (*a2amcp.Client)(nil)
)

// callWithReauth runs a tool call and, if the service answered "authenticate
// first", runs the handshake and retries the call exactly once.
//
// This lives below the middleware chain on purpose. writepath records a build_*
// result with writes.After, so letting an auth_required body reach it would
// register a payload that was never built and poison the build → sign →
// broadcast graph for the rest of the run. Retrying here means every layer above
// sees only the real result.
//
// Authenticating changes no trust boundary: the challenge is signed by the local
// signer, which refuses any text without the svpchain-mcp-auth-v1 prefix and a
// matching chain id, and the minted bearer authenticates the session — it
// authorizes no transfer. Every write still runs build → sign (whitelist-checked,
// user-confirmed) → broadcast afterwards.
func (env dispatchEnv) callWithReauth(
	ctx context.Context,
	name string,
	client authRetrier,
	call func(context.Context) (string, error),
) (string, error) {
	text, err := call(ctx)
	if !isAuthRequired(text, err) {
		return text, err
	}
	// Nothing to retry with: no transport that can authenticate, no key to sign
	// the challenge, or the handshake itself is what was refused.
	if client == nil || env.local == nil || isHandshakeTool(name) {
		return text, err
	}
	// The cached bearer is the one just refused, and EnsureAuth short-circuits
	// on BearerValid, so a stale-but-unexpired token would otherwise make the
	// retry fail exactly as the first call did.
	client.InvalidateBearer()
	if authErr := client.EnsureAuth(ctx, env.local.Owner(), env.local.SignChallenge); authErr != nil {
		// Fall back to the service's own answer, which carries the handshake
		// steps: the model can still complete it as a tool sequence. Reporting
		// the handshake failure instead would replace actionable instructions
		// with an internal error.
		return text, err
	}
	return call(ctx)
}
