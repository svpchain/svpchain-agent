package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/99designs/keyring"

	"github.com/svpchain/svpchain-agent/internal/agent/skills"
	"github.com/svpchain/svpchain-agent/internal/keystore"
	"github.com/svpchain/svpchain-agent/internal/manage"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

// StepKind classifies agent progress events.
type StepKind string

const (
	StepAuth   StepKind = "auth"
	StepTool   StepKind = "tool"
	StepThink  StepKind = "think"
	StepAnswer StepKind = "answer"
	StepError  StepKind = "error"
)

// Step is one progress update for the UI.
type Step struct {
	Kind   StepKind `json:"kind"`
	Title  string   `json:"title"`
	Detail string   `json:"detail,omitempty"`
}

// Config drives a single agent run.
type Config struct {
	ChainID   string
	RemoteURL string
	LLM       LLMConfig
	OnStep    func(Step)
}

const maxAgentIterations = 25

// Run executes one user message through the agent loop.
func Run(ctx context.Context, cfg Config, userMessage string) (string, error) {
	emit := func(s Step) {
		if cfg.OnStep != nil {
			cfg.OnStep(s)
		}
	}
	chainID := strings.TrimSpace(cfg.ChainID)
	if chainID == "" {
		return "", fmt.Errorf("chain id is required")
	}
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return "", fmt.Errorf("message is required")
	}

	var ring keyring.Keyring
	if r, err := keystore.Open(); err == nil {
		ring = r
	}
	hexKey, _, err := manage.SelectKey(ring, chainID, os.Getenv("SIGNER_KEY_HEX"))
	if err != nil {
		return "", err
	}
	priv, err := signer.ParsePrivKey(hexKey)
	if err != nil {
		return "", fmt.Errorf("parse key: %w", err)
	}
	evmID, _ := ParseEVMChainID(chainID)
	local := NewLocalSigner(priv, chainID, evmID)
	owner := local.Owner()

	remote := NewRemoteClient(cfg.RemoteURL)

	emit(Step{Kind: StepThink, Title: "Connecting to remote MCP…", Detail: cfg.RemoteURL})
	if err := remote.Connect(ctx); err != nil {
		return "", fmt.Errorf("connect remote mcp: %w", err)
	}
	defer remote.Close()

	emit(Step{Kind: StepAuth, Title: "Authenticating with remote MCP…"})
	if err := remote.EnsureAuth(ctx, owner, local.SignChallenge); err != nil {
		emit(Step{Kind: StepError, Title: "Authentication failed", Detail: err.Error()})
		return "", err
	}
	emit(Step{Kind: StepAuth, Title: "Authenticated", Detail: owner})

	tools, err := buildToolList(ctx, remote)
	if err != nil {
		return "", err
	}

	systemPrompt, err := skills.ComposeSystemPrompt(toolNames(tools))
	if err != nil {
		return "", fmt.Errorf("load agent skills: %w", err)
	}
	// Inject whitelist alias → address mappings so the assistant can resolve
	// "transfer to <alias>" without the user typing the raw address.
	if aliases := whitelistAliasPrompt(chainID); aliases != "" {
		systemPrompt += "\n\n" + aliases
	}

	llm := NewLLMClient(cfg.LLM)
	messages := []llmMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	for i := 0; i < maxAgentIterations; i++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		emit(Step{Kind: StepThink, Title: fmt.Sprintf("Thinking… (round %d)", i+1)})
		reply, err := llm.Chat(ctx, messages, tools)
		if err != nil {
			emit(Step{Kind: StepError, Title: "LLM error", Detail: err.Error()})
			return "", err
		}
		messages = append(messages, reply)

		if len(reply.ToolCalls) == 0 {
			answer := strings.TrimSpace(reply.Content)
			if answer == "" {
				answer = "(no response)"
			}
			emit(Step{Kind: StepAnswer, Title: "Done", Detail: answer})
			return answer, nil
		}

		for _, tc := range reply.ToolCalls {
			name := tc.Function.Name
			var args map[string]any
			if tc.Function.Arguments != "" {
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}
			emit(Step{Kind: StepTool, Title: "Calling " + name, Detail: truncate(tc.Function.Arguments, 4000)})

			result, callErr := dispatchTool(ctx, chainID, remote, local, name, args)
			if callErr != nil {
				// Fail fast: any tool error ends the run. There is no value in
				// feeding a failed call back to the LLM — it tends to loop or
				// guess. Whitelist rejections get a tailored message; every other
				// failure reports the tool and error, then stops.
				var rej *WhitelistRejection
				var answer string
				if errors.As(callErr, &rej) {
					answer = fmt.Sprintf("Transfer rejected — %s. No transaction was built, signed, or broadcast.", rej.Error())
				} else {
					answer = fmt.Sprintf("%s failed — %s. Stopped without further action.", name, callErr.Error())
				}
				emit(Step{Kind: StepError, Title: name + " failed", Detail: callErr.Error()})
				emit(Step{Kind: StepAnswer, Title: "Stopped", Detail: answer})
				return answer, nil
			}
			emit(Step{Kind: StepTool, Title: name + " ok", Detail: truncate(result, 4000)})
			messages = append(messages, llmMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       name,
				Content:    result,
			})
		}
	}
	return "", fmt.Errorf("agent exceeded %d tool rounds", maxAgentIterations)
}

func buildToolList(ctx context.Context, remote *RemoteClient) ([]llmTool, error) {
	remoteTools, err := remote.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]llmTool, 0, len(remoteTools)+len(LocalToolDefs()))
	for _, t := range remoteTools {
		if t == nil {
			continue
		}
		// Local sign_challenge is routed locally; remote auth tools stay on remote.
		out = append(out, llmTool{
			Type: "function",
			Function: llmFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	out = append(out, LocalToolDefs()...)
	return out, nil
}

func dispatchTool(ctx context.Context, chainID string, remote *RemoteClient, local *LocalSigner, name string, args map[string]any) (string, error) {
	// Whitelist gate: reject a transfer/approval to a non-whitelisted recipient
	// before the build_* call is forwarded — no build, sign, or broadcast happens.
	if err := checkWhitelistGate(chainID, name, args); err != nil {
		return "", err
	}
	if isHttpTool(name) {
		return HTTPFetchFromArgs(args)
	}
	if isLocalTool(name) {
		return local.CallTool(ctx, name, args)
	}
	return remote.CallTool(ctx, name, args)
}

func toolNames(tools []llmTool) []string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		if n := strings.TrimSpace(t.Function.Name); n != "" {
			names = append(names, n)
		}
	}
	return names
}
