package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/99designs/keyring"

	"github.com/svpchain/svpchain-agent/internal/agent/chainid"
	"github.com/svpchain/svpchain-agent/internal/agent/guard"
	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	localsigner "github.com/svpchain/svpchain-agent/internal/agent/local"
	"github.com/svpchain/svpchain-agent/internal/agent/memory"
	remotemcp "github.com/svpchain/svpchain-agent/internal/agent/remote"
	"github.com/svpchain/svpchain-agent/internal/agent/runlog"
	"github.com/svpchain/svpchain-agent/internal/agent/skills"
	"github.com/svpchain/svpchain-agent/internal/agent/step"
	"github.com/svpchain/svpchain-agent/internal/keystore"
	"github.com/svpchain/svpchain-agent/internal/manage"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

// ShutdownRemotePool closes pooled remote MCP sessions. Kept here so external callers
// (desktop, a2aserver) keep using agent.ShutdownRemotePool unchanged.
func ShutdownRemotePool() { remotemcp.Shutdown() }

// LLMConfig is the assistant's LLM settings. Aliased to llm.Config so external
// callers (desktop, a2aserver) keep using agent.LLMConfig unchanged.
type LLMConfig = llm.Config

// Step and StepKind are aliased from the leaf step package so external callers keep
// using agent.Step / agent.StepThink, while subpackages emit step.Step directly.
type (
	StepKind = step.Kind
	Step     = step.Step
)

const (
	StepAuth   = step.Auth
	StepTool   = step.Tool
	StepThink  = step.Think
	StepAnswer = step.Answer
	StepError  = step.Error
)

// Config drives a single agent run.
type Config struct {
	ChainID   string
	RemoteURL string
	LLM       LLMConfig
	OnStep    func(Step)
	// OnDelta, if set, receives assistant text increments as they stream in.
	OnDelta func(string)
	// RunLog, when enabled, appends a JSONL trace to the local run log file.
	RunLog *runlog.Recorder
}

const maxAgentIterations = 25

// Run executes one user message through the agent loop.
func Run(ctx context.Context, cfg Config, userMessage string) (answer string, err error) {
	var trace *runlog.Session
	if cfg.RunLog != nil && cfg.RunLog.Enabled() {
		trace = cfg.RunLog.Begin(runlog.Meta{
			ChainID:     cfg.ChainID,
			RemoteURL:   cfg.RemoteURL,
			Model:       cfg.LLM.Model,
			Provider:    cfg.LLM.Provider,
			UserMessage: userMessage,
		})
		defer func() { trace.Complete(answer, err) }()
	}

	emit := func(s Step) {
		if trace != nil {
			trace.RecordStep(string(s.Kind), s.Title, s.Detail)
		}
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
	if r, openErr := keystore.Open(); openErr == nil {
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
	evmID, _ := chainid.ParseEVM(chainID)
	local := localsigner.NewSigner(priv, chainID, evmID)
	owner := local.Owner()

	remote, err := remotemcp.Acquire(ctx, chainID, cfg.RemoteURL, owner, local.SignChallenge, emit)
	if err != nil {
		return "", fmt.Errorf("remote mcp: %w", err)
	}

	sessionMem, err := memory.Resolve(ctx, chainID, cfg.RemoteURL, owner, local, remote, emit)
	if err != nil {
		emit(Step{Kind: StepError, Title: "Session context failed", Detail: err.Error()})
		return "", err
	}

	tools, err := buildToolList(ctx, remote)
	if err != nil {
		return "", err
	}

	systemPrompt, err := skills.ComposeSystemPrompt(toolNames(tools))
	if err != nil {
		return "", fmt.Errorf("load agent skills: %w", err)
	}
	if block := memory.Prompt(sessionMem); block != "" {
		systemPrompt += "\n\n" + block
	}
	// Inject whitelist alias → address mappings so the assistant can resolve
	// "transfer to <alias>" without the user typing the raw address.
	if aliases := guard.AliasPrompt(chainID); aliases != "" {
		systemPrompt += "\n\n" + aliases
	}

	client := llm.NewClient(cfg.LLM)
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	for i := 0; i < maxAgentIterations; i++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if trace != nil {
			trace.SetRound(i + 1)
		}
		emit(Step{Kind: StepThink, Title: fmt.Sprintf("Thinking… (round %d)", i+1)})
		reply, err := client.Chat(ctx, messages, tools, cfg.OnDelta)
		if err != nil {
			emit(Step{Kind: StepError, Title: "LLM error", Detail: err.Error()})
			return "", err
		}
		messages = append(messages, reply)

		if len(reply.ToolCalls) == 0 {
			answer = strings.TrimSpace(reply.Content)
			if answer == "" {
				answer = "(no response)"
			}
			return answer, nil
		}

		for _, tc := range reply.ToolCalls {
			name := tc.Function.Name
			var args map[string]any
			if tc.Function.Arguments != "" {
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}
			emit(Step{Kind: StepTool, Title: "Calling " + name, Detail: truncate(tc.Function.Arguments, 4000)})

			var finish func(bool, string, string)
			if trace != nil {
				finish = trace.RecordTool(name, tc.Function.Arguments)
			} else {
				finish = func(bool, string, string) {}
			}

			result, callErr := dispatchTool(ctx, chainID, remote, local, name, args, &sessionMem)
			if callErr != nil {
				finish(false, "", callErr.Error())
				var rej *guard.Rejection
				if errors.As(callErr, &rej) {
					answer = fmt.Sprintf("Transfer rejected — %s. No transaction was built, signed, or broadcast.", rej.Error())
				} else {
					answer = fmt.Sprintf("%s failed — %s. Stopped without further action.", name, callErr.Error())
				}
				emit(Step{Kind: StepError, Title: name + " failed", Detail: callErr.Error()})
				emit(Step{Kind: StepAnswer, Title: "Stopped", Detail: answer})
				return answer, nil
			}
			finish(true, result, "")
			emit(Step{Kind: StepTool, Title: name + " ok", Detail: truncate(result, 4000)})
			messages = append(messages, llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       name,
				Content:    result,
			})
		}
	}
	return "", fmt.Errorf("agent exceeded %d tool rounds", maxAgentIterations)
}

// truncate shortens s for step/detail display (the llm package keeps its own copy).
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
