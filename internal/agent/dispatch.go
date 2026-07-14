package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/svpchain/svpchain-agent/internal/agent/a2acall"
	"github.com/svpchain/svpchain-agent/internal/agent/guard"
	"github.com/svpchain/svpchain-agent/internal/agent/httpfetch"
	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	localsigner "github.com/svpchain/svpchain-agent/internal/agent/local"
	"github.com/svpchain/svpchain-agent/internal/agent/memory"
	remotemcp "github.com/svpchain/svpchain-agent/internal/agent/remote"
	"github.com/svpchain/svpchain-agent/internal/agent/skills"
	"github.com/svpchain/svpchain-agent/internal/agent/x402"
)

// buildToolList merges remote MCP tool schemas with the local-only tool defs.
func buildToolList(ctx context.Context, remote *remotemcp.Client) ([]llm.Tool, error) {
	remoteTools, err := remote.ListTools(ctx)
	if err != nil {
		return nil, err
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
	return out, nil
}

// dispatchTool routes one tool call to its handler. It is the trust-boundary hop:
// the whitelist gate runs first (before any build_* is forwarded), then cached
// whoami short-circuits, then local/x402/http/a2a handlers, finally the remote MCP.
func dispatchTool(ctx context.Context, chainID string, remote *remotemcp.Client, local *localsigner.Signer, name string, args map[string]any, mem *memory.Session) (string, error) {
	// Whitelist gate: reject a transfer/approval to a non-whitelisted recipient
	// before the build_* call is forwarded — no build, sign, or broadcast happens.
	if err := guard.Check(chainID, name, args); err != nil {
		return "", err
	}
	if mem != nil {
		if cached, ok := mem.ToolResult(name); ok {
			return cached, nil
		}
	}
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
			return "", fmt.Errorf("unknown x402 tool %q", name)
		}
	}
	if a2acall.IsTool(name) {
		return a2acall.SendFromArgs(ctx, args)
	}
	if name == skills.ReferenceToolName {
		return skills.ReadReferenceFromArgs(args)
	}
	if localsigner.IsLocalTool(name) {
		result, err := local.CallTool(ctx, name, args)
		if err == nil && mem != nil && name == "signer_whoami" {
			mem.SetToolResult(name, result)
			_ = memory.Save(*mem)
		}
		return result, err
	}
	result, err := remote.CallTool(ctx, name, args)
	if err == nil && mem != nil && name == "whoami" {
		mem.SetToolResult(name, result)
		_ = memory.Save(*mem)
	}
	return result, err
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
