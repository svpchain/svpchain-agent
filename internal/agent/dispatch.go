package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/svpchain/svpchain-agent/internal/agent/a2acall"
	"github.com/svpchain/svpchain-agent/internal/agent/delegatecall"
	"github.com/svpchain/svpchain-agent/internal/agent/guard"
	"github.com/svpchain/svpchain-agent/internal/agent/httpfetch"
	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	localsigner "github.com/svpchain/svpchain-agent/internal/agent/local"
	"github.com/svpchain/svpchain-agent/internal/agent/memory"
	"github.com/svpchain/svpchain-agent/internal/agent/skills"
	"github.com/svpchain/svpchain-agent/internal/agent/x402"
)

// buildToolList returns local signer and Agent Hub/A2A tool definitions.
func buildToolList(deleg *delegatecall.Service) ([]llm.Tool, error) {
	out := make([]llm.Tool, 0, len(localsigner.ToolDefs()))
	out = append(out, localsigner.ToolDefs()...)
	out = append(out, deleg.ToolDefs()...)
	return out, nil
}

// dispatchTool routes one local or delegated tool call to its handler.
func dispatchTool(ctx context.Context, chainID string, local *localsigner.Signer, deleg *delegatecall.Service, name string, args map[string]any, mem *memory.Session) (string, error) {
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
	if delegatecall.IsTool(name) {
		return deleg.Call(ctx, name, args)
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
	return "", fmt.Errorf("unknown tool %q", name)
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
