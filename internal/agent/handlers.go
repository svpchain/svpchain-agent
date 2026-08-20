package agent

import (
	"context"

	"github.com/svpchain/svpchain-agent/internal/agent/a2acall"
	"github.com/svpchain/svpchain-agent/internal/agent/delegatecall"
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
		delegateHandler{svc: env.deleg},
		skillRefHandler{},
		localHandler{env: env},
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

type delegateHandler struct {
	svc *delegatecall.Service
}

func (h delegateHandler) Match(name string) bool { return delegatecall.IsTool(name) }

func (h delegateHandler) Call(ctx context.Context, name string, args map[string]any) (string, error) {
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

// remoteHandler is the catch-all: build_*/broadcast_* and other remote MCP tools.
// Match is always true, so this must stay last in handlers().
type remoteHandler struct {
	env dispatchEnv
}

func (remoteHandler) Match(string) bool { return true }

func (h remoteHandler) Call(ctx context.Context, name string, args map[string]any) (string, error) {
	if h.env.remote == nil {
		return "", errRemoteDisabled(name)
	}
	result, err := h.env.remote.CallTool(ctx, name, args)
	if err == nil && h.env.mem != nil && name == "whoami" {
		h.env.mem.SetToolResult(name, result)
		_ = memory.Save(*h.env.mem)
	}
	return result, err
}
