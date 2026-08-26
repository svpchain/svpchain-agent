package agent

import (
	"context"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/svpchain/svpchain-agent/internal/agent/discovery"
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
func buildToolList(ctx context.Context, remote *remotemcp.Client, disc *discovery.Service) ([]llm.Tool, error) {
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
	discTools := disc.ToolDefs()
	out = append(out, discTools...)
	// Finding an agent is only useful if its tools can then be attached, so the
	// two are offered together or not at all.
	if len(discTools) > 0 {
		out = append(out, ConnectToolDef())
	}
	return out, nil
}

// dispatchTool is the test-facing entry onto the middleware chain
// (observe → guard → writepath → cache → protocol mux).
func dispatchTool(ctx context.Context, chainID string, remote *remotemcp.Client, local *localsigner.Signer, disc *discovery.Service, confirm hitl.Func, writes *writepath.Tracker, name string, args map[string]any, mem *memory.Session) (string, error) {
	return dispatchEnv{
		chainID: chainID,
		remote:  remote,
		local:   local,
		disc:    disc,
		confirm: confirm,
		writes:  writes,
		mem:     mem,
	}.dispatch(ctx, name, args)
}

// errRemoteDisabled answers a name that reached the catch-all with no remote
// MCP behind it. The name is usually NOT a remote tool — the catch-all also
// collects names nothing serves — so this must not assert that it is one, or a
// model that invented a tool is told to go enable a setting that would not have
// helped, and stops on a task it could still finish.
func errRemoteDisabled(name string) error {
	return fmt.Errorf(
		"%q is not available in this conversation. The remote MCP is disabled in "+
			"Settings, so its tools — build_*, broadcast_*, market data — are switched "+
			"off; a name that is not one of those does not exist at all. Either way do not retry it "+
			"as-is; if it came from an agent attached in an earlier turn, call "+
			"a2a_connect_agent with that agent_url again; the tools an attached agent "+
			"adds are exactly the ones listed in that result.", name,
	)
}

// errAgentLost answers a name that reached the catch-all after the agent
// attached in an earlier turn failed to re-attach this run. The model listed
// its tools from that turn, so the useful instruction is to reconnect — not
// the remote-MCP advice, which would not bring these tools back.
func errAgentLost(name, url string, cause error) error {
	return fmt.Errorf(
		"%q is not available right now: the agent at %s attached earlier in this conversation "+
			"could not be re-attached (%v). Call %s with that agent_url again to restore its tools, "+
			"then retry.", name, url, cause, ConnectTool,
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
