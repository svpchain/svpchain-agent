package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/svpchain/svpchain-agent/internal/agent/a2acall"
	"github.com/svpchain/svpchain-agent/internal/agent/a2amcp"
	"github.com/svpchain/svpchain-agent/internal/agent/discovery"
	"github.com/svpchain/svpchain-agent/internal/agent/hitl"
	"github.com/svpchain/svpchain-agent/internal/agent/httpfetch"
	localsigner "github.com/svpchain/svpchain-agent/internal/agent/local"
	"github.com/svpchain/svpchain-agent/internal/agent/memory"
	"github.com/svpchain/svpchain-agent/internal/agent/skills"
	"github.com/svpchain/svpchain-agent/internal/agent/x402"
)

// toolHandler is one protocol's tool surface. The mux picks the first Match;
// remoteHandler is last so a local sign_* never reaches the remote MCP.
type toolHandler interface {
	Match(name string) bool
	Call(ctx context.Context, name string, args map[string]any) (string, error)
}

func (env dispatchEnv) handlers() []toolHandler {
	return []toolHandler{
		httpHandler{},
		x402Handler{},
		a2aHandler{},
		connectHandler{env: env},
		paidSettlementHandler{env: env},
		discoverHandler{svc: env.disc},
		skillRefHandler{},
		localHandler{env: env},
		attachedHandler{env: env},
		remoteHandler{env: env},
	}
}

func (env dispatchEnv) mux(ctx context.Context, name string, args map[string]any) (string, error) {
	for _, h := range env.handlers() {
		if h.Match(name) {
			return h.Call(ctx, name, args)
		}
	}
	return "", errUnknownTool(name)
}

type httpHandler struct{}

func (httpHandler) Match(name string) bool { return httpfetch.IsTool(name) }

func (httpHandler) Call(_ context.Context, _ string, args map[string]any) (string, error) {
	return httpfetch.FromArgs(args)
}

type x402Handler struct{}

func (x402Handler) Match(name string) bool { return x402.IsTool(name) }

func (x402Handler) Call(_ context.Context, name string, args map[string]any) (string, error) {
	switch name {
	case "x402_prepare_typed_data":
		return x402.PrepareFromArgs(args)
	case "x402_build_payment":
		return x402.BuildPaymentFromArgs(args)
	default:
		return "", errUnknownX402(name)
	}
}

type a2aHandler struct{}

func (a2aHandler) Match(name string) bool { return a2acall.IsTool(name) }

func (a2aHandler) Call(ctx context.Context, _ string, args map[string]any) (string, error) {
	return a2acall.SendFromArgs(ctx, args)
}

type skillRefHandler struct{}

func (skillRefHandler) Match(name string) bool { return name == skills.ReferenceToolName }

func (skillRefHandler) Call(_ context.Context, _ string, args map[string]any) (string, error) {
	return skills.ReadReferenceFromArgs(args)
}

type discoverHandler struct {
	svc *discovery.Service
}

func (h discoverHandler) Match(name string) bool { return discovery.IsTool(name) }

func (h discoverHandler) Call(ctx context.Context, name string, args map[string]any) (string, error) {
	return h.svc.Call(ctx, name, args)
}

type localHandler struct {
	env dispatchEnv
}

func (h localHandler) Match(name string) bool { return localsigner.IsLocalTool(name) }

func (h localHandler) Call(ctx context.Context, name string, args map[string]any) (string, error) {
	if hitl.NeedsConfirm(name) {
		if err := hitl.Ask(ctx, h.env.confirm, hitl.SignRequest(name, args)); err != nil {
			return "", err
		}
	}
	result, err := h.env.local.CallTool(ctx, name, args)
	if err == nil && h.env.mem != nil && name == "signer_whoami" {
		h.env.mem.SetToolResult(name, result)
		_ = memory.Save(*h.env.mem)
	}
	return result, err
}

// connectHandler attaches a discovered agent's tool surface to this run.
type connectHandler struct {
	env dispatchEnv
}

// paidSettlementHandler funds and assigns the selected market agent before
// its execution tools are used in this run.
type paidSettlementHandler struct {
	env dispatchEnv
}

func (h paidSettlementHandler) Match(name string) bool {
	return h.env.paid != nil && name == BeginSettlementTool
}

func (h paidSettlementHandler) Call(ctx context.Context, _ string, args map[string]any) (string, error) {
	return h.env.paid.Start(ctx, args, func(ctx context.Context, endpoint string) (string, error) {
		return h.env.connect(ctx, map[string]any{"agent_url": endpoint})
	})
}

func (connectHandler) Match(name string) bool { return name == ConnectTool }

func (h connectHandler) Call(ctx context.Context, _ string, args map[string]any) (string, error) {
	if h.env.paid == nil {
		return h.env.connect(ctx, args)
	}
	endpoint, _ := args["agent_url"].(string)
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("agent_url is required")
	}
	return h.env.paid.StartEndpoint(ctx, endpoint, func(ctx context.Context, endpoint string) (string, error) {
		return h.env.connect(ctx, map[string]any{"agent_url": endpoint})
	})
}

// attachedHandler routes a tool an attached A2A agent advertised. It sits after
// localHandler and matches only names that survived the precedence filter in
// attached.set, so it can never take a local or remote MCP tool — but it must
// stay ahead of remoteHandler, whose Match is unconditional.
type attachedHandler struct {
	env dispatchEnv
}

func (h attachedHandler) Match(name string) bool { return h.env.att.handles(name) }

func (h attachedHandler) Call(ctx context.Context, name string, args map[string]any) (string, error) {
	if h.env.paid != nil {
		endpoint := h.env.att.endpoint()
		if err := h.env.paid.EnsureEndpoint(ctx, endpoint, func(ctx context.Context, endpoint string) (string, error) {
			return h.env.connect(ctx, map[string]any{"agent_url": endpoint})
		}); err != nil {
			return "", err
		}
	}
	return h.env.att.call(ctx, name, args)
}

// remoteHandler is the catch-all: build_*/broadcast_* and other remote MCP tools.
// Match is always true, so this must stay last in handlers().
type remoteHandler struct {
	env dispatchEnv
}

func (remoteHandler) Match(string) bool { return true }

func (h remoteHandler) Call(ctx context.Context, name string, args map[string]any) (string, error) {
	if h.env.remote == nil {
		if h.env.canResumePaidTool(name) {
			return h.env.resumePaidTool(ctx, name, args)
		}
		if lostURL, lostErr := h.env.att.lost(); lostURL != "" {
			return "", errAgentLost(name, lostURL, lostErr)
		}
		return "", errRemoteDisabled(name)
	}
	result, err := h.env.remote.CallTool(ctx, name, args)
	if err == nil && h.env.mem != nil && name == "whoami" {
		h.env.mem.SetToolResult(name, result)
		_ = memory.Save(*h.env.mem)
	}
	return result, err
}

func (env dispatchEnv) canResumePaidTool(name string) bool {
	if env.paid == nil || strings.TrimSpace(env.resumeAgentURL) == "" {
		return false
	}
	_, ok := env.resumeAgentTools[strings.TrimSpace(name)]
	return ok
}

// resumePaidTool handles an explicit follow-up that names a capability the
// current conversation's market agent published in an earlier turn. Probe the
// Agent Card before collecting another payment, then create a fresh task and
// attach it for this behavior.
func (env dispatchEnv) resumePaidTool(ctx context.Context, name string, args map[string]any) (string, error) {
	endpoint := strings.TrimSpace(env.resumeAgentURL)
	candidate := a2amcp.New(endpoint)
	if err := candidate.Connect(ctx); err != nil {
		return "", fmt.Errorf("check saved market agent %s: %w", endpoint, err)
	}
	if !env.att.mayServe(candidate, name) {
		return "", fmt.Errorf("saved market agent %s no longer publishes %q; search Agent Market for a current agent", endpoint, name)
	}
	if _, err := env.paid.StartEndpoint(ctx, endpoint, func(ctx context.Context, endpoint string) (string, error) {
		return env.connect(ctx, map[string]any{"agent_url": endpoint})
	}); err != nil {
		return "", fmt.Errorf("renew settlement for saved market agent: %w", err)
	}
	return env.att.call(ctx, name, args)
}
