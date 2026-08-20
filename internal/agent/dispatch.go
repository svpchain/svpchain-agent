package agent

import (
	"context"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/svpchain/svpchain-agent/internal/agent/delegatecall"
	"github.com/svpchain/svpchain-agent/internal/agent/hitl"
	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	localsigner "github.com/svpchain/svpchain-agent/internal/agent/local"
	"github.com/svpchain/svpchain-agent/internal/agent/memory"
	remotemcp "github.com/svpchain/svpchain-agent/internal/agent/remote"
	"github.com/svpchain/svpchain-agent/internal/agent/writepath"
)

// buildToolList merges remote MCP tool schemas with the local-only tool defs.
// A nil remote means the remote MCP is switched off: the assistant then has
// only the local tools, and the skills that gate on remote tool names drop out
// of the system prompt on their own.
func buildToolList(ctx context.Context, remote *remotemcp.Client, deleg *delegatecall.Service) ([]llm.Tool, error) {
	var remoteTools []*mcpsdk.Tool // schemas the remote advertises; nil when off
	if remote != nil {
		var err error
		remoteTools, err = remote.ListTools(ctx)
		if err != nil {
			return nil, err
		}
	}
	out := make([]llm.Tool, 0, len(remoteTools)+len(localsigner.ToolDefs()))
	for _, t := range remoteTools {
		if t == nil {
			continue
		}
		// Local sign_challenge is routed locally; remote auth tools stay on remote.
		out = append(out, llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	out = append(out, localsigner.ToolDefs()...)
	out = append(out, deleg.ToolDefs()...)
	return out, nil
}

// dispatchTool is the test-facing entry onto the middleware chain
// (observe → guard → writepath → cache → protocol mux).
func dispatchTool(ctx context.Context, chainID string, remote *remotemcp.Client, local *localsigner.Signer, deleg *delegatecall.Service, confirm hitl.Func, writes *writepath.Tracker, name string, args map[string]any, mem *memory.Session) (string, error) {
	return dispatchEnv{
		chainID: chainID,
		remote:  remote,
		local:   local,
		deleg:   deleg,
		confirm: confirm,
		writes:  writes,
		mem:     mem,
	}.dispatch(ctx, name, args)
}

func errRemoteDisabled(name string) error {
	return fmt.Errorf(
		"%q is a remote MCP tool and the remote MCP is disabled in Settings — "+
			"it is unavailable for this conversation", name,
	)
}

func errUnknownX402(name string) error {
	return fmt.Errorf("unknown x402 tool %q", name)
}

func errUnknownTool(name string) error {
	return fmt.Errorf("unknown tool %q", name)
}

// toolNames lists the tool names available this run (used to gate skills).
func toolNames(tools []llm.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		if n := strings.TrimSpace(t.Function.Name); n != "" {
			names = append(names, n)
		}
	}
	return names
}
