package agent

import (
	"context"

	"github.com/svpchain/svpchain-agent/internal/agent/a2acall"
	"github.com/svpchain/svpchain-agent/internal/agent/delegatecall"
	"github.com/svpchain/svpchain-agent/internal/agent/guard"
	"github.com/svpchain/svpchain-agent/internal/agent/hitl"
	"github.com/svpchain/svpchain-agent/internal/agent/httpfetch"
	localsigner "github.com/svpchain/svpchain-agent/internal/agent/local"
	"github.com/svpchain/svpchain-agent/internal/agent/memory"
	remotemcp "github.com/svpchain/svpchain-agent/internal/agent/remote"
	"github.com/svpchain/svpchain-agent/internal/agent/skills"
	"github.com/svpchain/svpchain-agent/internal/agent/writepath"
	"github.com/svpchain/svpchain-agent/internal/agent/x402"
)

// toolFunc handles one tool call. Middleware wraps this the way Eino/Genkit
// wrap handlers: each layer can refuse before the next runs, and observe the
// result after.
type toolFunc func(ctx context.Context, name string, args map[string]any) (string, error)

type middleware func(toolFunc) toolFunc

func wrap(h toolFunc, mws ...middleware) toolFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// dispatchEnv is everything one tool call needs. Built once per Run so the
// write-path tracker outlives a single LLM round.
type dispatchEnv struct {
	chainID string
	remote  *remotemcp.Client
	local   *localsigner.Signer
	deleg   *delegatecall.Service
	confirm hitl.Func
	writes  *writepath.Tracker
	mem     *memory.Session
}

func (env dispatchEnv) dispatch(ctx context.Context, name string, args map[string]any) (string, error) {
	// Outer → inner: whitelist, then write-path graph, then whoami cache, then route.
	// A dialog (HITL) sits inside route so a whitelist/graph rejection never
	// asks the user to approve a forbidden action.
	h := wrap(env.route,
		env.withGuard(),
		env.withWritePath(),
		env.withCache(),
	)
	return h(ctx, name, args)
}

func (env dispatchEnv) withGuard() middleware {
	return func(next toolFunc) toolFunc {
		return func(ctx context.Context, name string, args map[string]any) (string, error) {
			if err := guard.Check(env.chainID, name, args); err != nil {
				return "", err
			}
			return next(ctx, name, args)
		}
	}
}

func (env dispatchEnv) withWritePath() middleware {
	return func(next toolFunc) toolFunc {
		return func(ctx context.Context, name string, args map[string]any) (string, error) {
			if err := env.writes.Before(name, args); err != nil {
				return "", err
			}
			result, err := next(ctx, name, args)
			if err != nil {
				return "", err
			}
			if err := env.writes.After(name, args, result); err != nil {
				return "", err
			}
			return result, nil
		}
	}
}

func (env dispatchEnv) withCache() middleware {
	return func(next toolFunc) toolFunc {
		return func(ctx context.Context, name string, args map[string]any) (string, error) {
			if env.mem != nil {
				if cached, ok := env.mem.ToolResult(name); ok {
					return cached, nil
				}
			}
			return next(ctx, name, args)
		}
	}
}

func (env dispatchEnv) route(ctx context.Context, name string, args map[string]any) (string, error) {
	if httpfetch.IsTool(name) {
		return httpfetch.FromArgs(args)
	}
	if x402.IsTool(name) {
		switch name {
		case "x402_prepare_typed_data":
			return x402.PrepareFromArgs(args)
		case "x402_build_payment":
			return x402.BuildPaymentFromArgs(args)
		default:
			return "", errUnknownX402(name)
		}
	}
	if a2acall.IsTool(name) {
		return a2acall.SendFromArgs(ctx, args)
	}
	if delegatecall.IsTool(name) {
		return env.deleg.Call(ctx, name, args)
	}
	if name == skills.ReferenceToolName {
		return skills.ReadReferenceFromArgs(args)
	}
	if localsigner.IsLocalTool(name) {
		if hitl.NeedsConfirm(name) {
			if err := hitl.Ask(ctx, env.confirm, hitl.SignRequest(name, args)); err != nil {
				return "", err
			}
		}
		result, err := env.local.CallTool(ctx, name, args)
		if err == nil && env.mem != nil && name == "signer_whoami" {
			env.mem.SetToolResult(name, result)
			_ = memory.Save(*env.mem)
		}
		return result, err
	}
	if env.remote == nil {
		return "", errRemoteDisabled(name)
	}
	result, err := env.remote.CallTool(ctx, name, args)
	if err == nil && env.mem != nil && name == "whoami" {
		env.mem.SetToolResult(name, result)
		_ = memory.Save(*env.mem)
	}
	return result, err
}
