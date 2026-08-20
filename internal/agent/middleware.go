package agent

import (
	"context"
	"encoding/json"

	"github.com/svpchain/svpchain-agent/internal/agent/delegatecall"
	"github.com/svpchain/svpchain-agent/internal/agent/guard"
	"github.com/svpchain/svpchain-agent/internal/agent/hitl"
	localsigner "github.com/svpchain/svpchain-agent/internal/agent/local"
	"github.com/svpchain/svpchain-agent/internal/agent/memory"
	remotemcp "github.com/svpchain/svpchain-agent/internal/agent/remote"
	"github.com/svpchain/svpchain-agent/internal/agent/writepath"
)

// toolFunc handles one tool call. Middleware wraps this the way Eino/Genkit
// wrap handlers: each layer can refuse before the next runs, and observe the
// result after. Middleware never executes a tool.
type toolFunc func(ctx context.Context, name string, args map[string]any) (string, error)

type middleware func(toolFunc) toolFunc

func wrap(h toolFunc, mws ...middleware) toolFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// ToolObserver records one tool invocation. *runlog.Session implements this;
// an OpenTelemetry span wrapper plugs in here without changing the handler chain.
// It is telemetry only — it must not execute tools or override guard.Check.
type ToolObserver interface {
	RecordTool(name, args string) func(ok bool, result, errDetail string)
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
	observe ToolObserver
}

func (env dispatchEnv) dispatch(ctx context.Context, name string, args map[string]any) (string, error) {
	// Outer → inner. Observe is telemetry (runlog today; OTel later) and never
	// executes a tool. Guard is the first layer that can refuse — empty
	// whitelist still rejects every transfer. HITL sits inside the local-signer
	// handler so a whitelist/graph rejection never opens a dialog.
	h := wrap(env.mux,
		env.withObserve(),
		env.withGuard(),
		env.withWritePath(),
		env.withCache(),
	)
	return h(ctx, name, args)
}

func (env dispatchEnv) withObserve() middleware {
	return func(next toolFunc) toolFunc {
		return func(ctx context.Context, name string, args map[string]any) (string, error) {
			finish := func(bool, string, string) {}
			if env.observe != nil {
				finish = env.observe.RecordTool(name, toolArgsJSON(args))
			}
			result, err := next(ctx, name, args)
			if err != nil {
				finish(false, "", err.Error())
				return "", err
			}
			finish(true, result, "")
			return result, nil
		}
	}
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

func toolArgsJSON(args map[string]any) string {
	if args == nil {
		return ""
	}
	bz, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	return string(bz)
}
