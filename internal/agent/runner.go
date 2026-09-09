package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/99designs/keyring"

	"github.com/svpchain/svpchain-agent/internal/agent/chainid"
	"github.com/svpchain/svpchain-agent/internal/agent/discovery"
	"github.com/svpchain/svpchain-agent/internal/agent/guard"
	"github.com/svpchain/svpchain-agent/internal/agent/history"
	"github.com/svpchain/svpchain-agent/internal/agent/hitl"
	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	localsigner "github.com/svpchain/svpchain-agent/internal/agent/local"
	"github.com/svpchain/svpchain-agent/internal/agent/memory"
	"github.com/svpchain/svpchain-agent/internal/agent/phoenix"
	remotemcp "github.com/svpchain/svpchain-agent/internal/agent/remote"
	"github.com/svpchain/svpchain-agent/internal/agent/runlog"
	agentsettlement "github.com/svpchain/svpchain-agent/internal/agent/settlement"
	"github.com/svpchain/svpchain-agent/internal/agent/skills"
	"github.com/svpchain/svpchain-agent/internal/agent/step"
	"github.com/svpchain/svpchain-agent/internal/agent/writepath"
	"github.com/svpchain/svpchain-agent/internal/agentmarket"
	"github.com/svpchain/svpchain-agent/internal/chainrpc"
	"github.com/svpchain/svpchain-agent/internal/keystore"
	"github.com/svpchain/svpchain-agent/internal/manage"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

// ShutdownRemotePool closes pooled remote MCP sessions. Kept here so external callers
// (desktop) keep using agent.ShutdownRemotePool unchanged.
func ShutdownRemotePool() { remotemcp.Shutdown() }

// LLMConfig is the assistant's LLM settings. Aliased to llm.Config so external
// callers (desktop) keep using agent.LLMConfig unchanged.
type LLMConfig = llm.Config

// Step and StepKind are aliased from the leaf step package so external callers keep
// using agent.Step / agent.StepThink, while subpackages emit step.Step directly.
type (
	StepKind = step.Kind
	Step     = step.Step
)

const (
	StepAuth    = step.Auth
	StepTool    = step.Tool
	StepThink   = step.Think
	StepAnswer  = step.Answer
	StepError   = step.Error
	StepConfirm = step.Confirm
)

// Config drives a single agent run.
type Config struct {
	ChainID   string
	RemoteURL string
	// AgentMarketURL is the SVP Agent Market service's base URL, the backend
	// for semantic agent search (search_agents). Empty falls back to
	// agentmarket.DefaultURL.
	AgentMarketURL string
	// ChainRPCURL is the CometBFT RPC base used to look up broadcast tx
	// hashes (GET /tx?hash=0x…). Empty falls back to chainrpc.URLForChain.
	ChainRPCURL string
	// Confirm is the HITL hook for local sign_* calls.
	// Nil, a decline, or a timeout all deny — nothing is signed.
	Confirm hitl.Func
	LLM     LLMConfig
	OnStep  func(Step)
	// OnDelta, if set, receives assistant text increments as they stream in.
	OnDelta func(string)
	// RunLog, when enabled, appends a JSONL trace to the local run log file.
	RunLog *runlog.Recorder
	// PhoenixOTLPURL, when set, exports redacted OpenInference spans to a
	// Phoenix OTLP HTTP endpoint. Empty disables export.
	PhoenixOTLPURL string
	// Prior is earlier conversation turns (no system message) prepended before
	// the current user message, enabling multi-turn context.
	Prior []llm.Message
	// OnTranscript, if set, receives the messages this run added (the user
	// message, assistant replies, and tool round-trips) with tool-call pairing
	// already repaired, so the caller can persist them as history.
	OnTranscript func(runID string, msgs []llm.Message)
	// SessionID / SessionTitle attach this run to a multi-turn history
	// conversation so the GUI can jump from a trace back to the chat.
	SessionID    string
	SessionTitle string
	// AttachedAgentURL is an A2A agent endpoint attached earlier in this
	// conversation. The run re-attaches it before the first LLM round so the
	// agent's tools stay callable across user messages; a failed re-attach is
	// reported as a step and the run continues without it.
	AttachedAgentURL string
	// AttachedAgentTools is the filtered tool list that the previously attached
	// market agent published. It lets an explicit follow-up safely resume the
	// exact capability after a fresh settlement.
	AttachedAgentTools []string
	// OnAttach, if set, is called with the endpoint each time a2a_connect_agent
	// attaches an agent, so the caller can persist it for the next run.
	OnAttach func(url string, tools []string)
	// Settlement is an optional, explicitly assigned AgentSettlement task for
	// one user-requested behavior. It reports that behavior's final successful
	// broadcast hash to agent-validator when the run ends. The TaskID must be
	// the settlement contract bytes32 task ID, never an A2A task ID. Nil leaves
	// generic chat runs unchanged.
	Settlement *agentsettlement.Config
	// SettlementValidatorURL enables user-funded agent execution from the chat
	// loop. It is intentionally separate from Settlement, which is retained for
	// trusted callers that already assigned a task.
	SettlementValidatorURL string
	SettlementRPCURL       string
}

const maxAgentIterations = 25

// *runlog.Session is the shipped ToolObserver; Phoenix satisfies the same interface.
var (
	_ ToolObserver = (*runlog.Session)(nil)
	_ ToolObserver = (*phoenix.Session)(nil)
)

// Run executes one user message through the agent loop.
func Run(ctx context.Context, cfg Config, userMessage string) (answer string, err error) {
	settlementReporter := agentsettlement.NewDeferred()
	var emit func(Step)
	if cfg.Settlement != nil {
		if err = settlementReporter.Configure(*cfg.Settlement); err != nil {
			return "", err
		}
	}
	defer func() {
		reportCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 12*time.Second)
		reportErr := settlementReporter.Report(reportCtx)
		cancel()
		if reportErr != nil && err == nil {
			err = fmt.Errorf("settlement callback: %w", reportErr)
			return
		}
		if reportErr == nil && emit != nil {
			if taskID, txHash, ok := settlementReporter.Callback(); ok {
				answer = strings.TrimSpace(answer) + "\n\n✅ Agent Validator callback succeeded" +
					"\n- task_id: " + taskID +
					"\n- execution tx_hash: " + txHash
				emit(Step{
					Kind:   StepTool,
					Title:  "Agent Validator callback succeeded",
					Detail: "task_id=" + taskID + " tx_hash=" + txHash,
				})
			}
		}
	}()
	var trace *runlog.Session
	var ph *phoenix.Session
	if cfg.RunLog != nil && cfg.RunLog.Enabled() {
		trace = cfg.RunLog.Begin(runlog.Meta{
			ChainID:      cfg.ChainID,
			RemoteURL:    cfg.RemoteURL,
			Model:        cfg.LLM.Model,
			Provider:     cfg.LLM.Provider,
			UserMessage:  userMessage,
			SessionID:    cfg.SessionID,
			SessionTitle: cfg.SessionTitle,
		})
		defer func() {
			lookupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			trace.VerifyTxs(lookupCtx, chainrpc.Lookup(resolveChainRPC(cfg)))
			cancel()
			trace.Complete(answer, err)
		}()
	}
	ph = phoenix.Begin(cfg.PhoenixOTLPURL, phoenix.Meta{
		RunID:        runlogID(trace),
		SessionID:    cfg.SessionID,
		SessionTitle: cfg.SessionTitle,
		ChainID:      cfg.ChainID,
		Model:        cfg.LLM.Model,
		Provider:     cfg.LLM.Provider,
		UserMessage:  userMessage,
	})
	if ph != nil {
		defer func() {
			ph.End(answer, err)
			flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			_ = ph.Flush(flushCtx)
			cancel()
		}()
	}

	emit = func(s Step) {
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

	// An empty RemoteURL switches the remote MCP off entirely — no connection,
	// no authentication, no remote tools offered to the model. Useful for
	// working against the chain alone, and the only way to be certain nothing
	// leaves the machine except what the discovery tools read.
	var remote *remotemcp.Client
	if strings.TrimSpace(cfg.RemoteURL) != "" {
		remote, err = remotemcp.Acquire(ctx, chainID, cfg.RemoteURL, owner, local.SignChallenge, emit)
		if err != nil {
			return "", fmt.Errorf("remote mcp: %w", err)
		}
	} else {
		emit(Step{Kind: StepThink, Title: "Direct execution is disabled — use Agent Market to find an agent"})
	}

	sessionMem, err := memory.Resolve(ctx, chainID, cfg.RemoteURL, owner, local, remote, emit)
	if err != nil {
		emit(Step{Kind: StepError, Title: "Session context failed", Detail: err.Error()})
		return "", err
	}

	confirm := cfg.Confirm
	if confirm != nil {
		orig := confirm
		confirm = func(ctx context.Context, req hitl.Request) bool {
			emit(Step{Kind: StepConfirm, Title: "Waiting for confirmation…", Detail: req.Title})
			return orig(ctx, req)
		}
	}
	market := agentmarket.New(cfg.AgentMarketURL)
	disc := &discovery.Service{Market: market}
	writes := writepath.New()
	baseTools, err := buildToolList(ctx, remote, disc)
	if err != nil {
		return "", err
	}
	var paid *paidAgentFlow
	if strings.TrimSpace(cfg.SettlementValidatorURL) != "" {
		paid, err = newPaidAgentFlow(cfg.SettlementValidatorURL, cfg.SettlementRPCURL, chainID, priv, confirm, market, settlementReporter)
		if err != nil {
			return "", err
		}
		disc.PaymentToken = paid.PaymentToken
		baseTools = append(baseTools, paid.ToolDef())
	}
	// Tools an attached A2A agent adds mid-run (see attach.go). The base list
	// is the precedence floor: nothing attached may take a name already here.
	att := newAttached(baseTools)
	att.onAttach = cfg.OnAttach
	tools := baseTools
	resumeTools := toolNameSet(cfg.AttachedAgentTools)
	if len(resumeTools) == 0 && strings.TrimSpace(cfg.AttachedAgentURL) != "" {
		// Sessions created before AttachedAgentTools was persisted can still
		// resume a known capability. The Agent Card preflight in resumePaidTool
		// remains the authority before any new settlement is funded.
		resumeTools = historicalAgentToolNames(cfg.Prior, baseTools)
	}

	observers := []ToolObserver{trace, ph}
	if settlementReporter != nil {
		observers = append(observers, settlementReporter)
	}

	env := dispatchEnv{
		chainID:          chainID,
		remote:           remote,
		local:            local,
		disc:             disc,
		confirm:          confirm,
		writes:           writes,
		att:              att,
		paid:             paid,
		resumeAgentURL:   strings.TrimSpace(cfg.AttachedAgentURL),
		resumeAgentTools: resumeTools,
		mem:              &sessionMem,
		observe:          composeObservers(observers...),
	}

	// composePrompt is used again if an attach changes the tool set, so the
	// skills gated on what was just attached actually reach the model.
	composePrompt := func(ts []llm.Tool) (string, []string, error) {
		prompt, names, err := skills.Compose(toolNames(ts))
		if err != nil {
			return "", nil, fmt.Errorf("load agent skills: %w", err)
		}
		if block := memory.Prompt(sessionMem); block != "" {
			prompt += "\n\n" + block
		}
		// Inject whitelist alias → address mappings so the assistant can resolve
		// "transfer to <alias>" without the user typing the raw address.
		if aliases := guard.AliasPrompt(chainID); aliases != "" {
			prompt += "\n\n" + aliases
		}
		return prompt, names, nil
	}

	// A paid market agent is funded for one user-requested behavior, so it must
	// be selected and funded again on a later user message. Do not revive an old
	// attachment and let a new behavior reuse it without a fresh settlement task.
	if u := strings.TrimSpace(cfg.AttachedAgentURL); u != "" {
		if paid != nil {
			emit(Step{Kind: StepThink, Title: "A fresh settlement is required before reusing agent " + u})
		} else {
			emit(Step{Kind: StepThink, Title: "Re-attaching agent " + u})
			if _, aerr := env.connect(ctx, map[string]any{"agent_url": u}); aerr != nil {
				att.noteReattachFailure(u, aerr)
				emit(Step{Kind: StepError, Title: "Re-attach failed", Detail: aerr.Error()})
			} else {
				tools = append(append([]llm.Tool(nil), baseTools...), att.toolDefs()...)
			}
		}
	}

	systemPrompt, skillNames, err := composePrompt(tools)
	if err != nil {
		return "", err
	}
	promptHash := runlog.PromptSHA256(systemPrompt)
	if trace != nil {
		trace.SetPrompt(promptHash, skillNames)
	}
	if ph != nil {
		ph.SetPrompt(promptHash, skillNames)
	}

	client := llm.NewClient(cfg.LLM)
	messages := make([]llm.Message, 0, len(cfg.Prior)+2)
	messages = append(messages, llm.Message{Role: "system", Content: systemPrompt})
	messages = append(messages, cfg.Prior...)
	messages = append(messages, llm.Message{Role: "user", Content: userMessage})

	// Persist this run's new messages (everything after system + prior) so the
	// caller can carry the conversation into the next turn. Runs on every exit
	// path once the transcript exists, including errors and cancellation.
	transcriptBase := 1 + len(cfg.Prior)
	if cfg.OnTranscript != nil {
		defer func() {
			newMsgs := append([]llm.Message(nil), messages[transcriptBase:]...)
			if err != nil {
				// Keep roles alternating so the persisted transcript stays valid.
				newMsgs = append(newMsgs, llm.Message{Role: "assistant", Content: "(run failed: " + err.Error() + ")"})
			} else if answer != "" {
				last := &newMsgs[len(newMsgs)-1]
				if last.Role == "assistant" && len(last.ToolCalls) == 0 {
					// The final streamed reply is this answer (possibly untrimmed);
					// normalize instead of appending a duplicate assistant message.
					last.Content = answer
				} else {
					newMsgs = append(newMsgs, llm.Message{Role: "assistant", Content: answer})
				}
			}
			cfg.OnTranscript(runlogID(trace), history.RepairPairing(newMsgs))
		}()
	}

	for i := 0; i < maxAgentIterations; i++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if trace != nil {
			trace.SetRound(i + 1)
		}
		// An attach mid-run grows the tool list and can change which skills
		// apply; both must be in place before the next round is composed.
		if added := att.toolDefs(); len(added) != len(tools)-len(baseTools) {
			tools = append(append([]llm.Tool(nil), baseTools...), added...)
			if prompt, names, perr := composePrompt(tools); perr == nil {
				messages[0].Content = prompt
				if trace != nil {
					trace.SetPrompt(runlog.PromptSHA256(prompt), names)
				}
			}
		}

		emit(Step{Kind: StepThink, Title: fmt.Sprintf("Thinking… (round %d)", i+1)})
		llmResult, err := client.Chat(ctx, messages, tools, cfg.OnDelta)
		if err != nil {
			emit(Step{Kind: StepError, Title: "LLM error", Detail: err.Error()})
			return "", err
		}
		if trace != nil {
			trace.RecordLLMRound(i+1, llmResult)
		}
		if ph != nil {
			ph.RecordLLM(i+1, llmResult)
		}
		reply := llmResult.Message
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

			result, callErr := env.dispatch(ctx, name, args)
			if callErr != nil {
				var denied *hitl.Denied
				var rej *guard.Rejection
				var broken *writepath.Violation
				if errors.As(callErr, &denied) {
					answer = denied.StopMessage()
				} else if errors.As(callErr, &rej) {
					answer = fmt.Sprintf("Transfer rejected — %s. No transaction was built, signed, or broadcast.", rej.Error())
				} else if errors.As(callErr, &broken) {
					answer = broken.StopMessage()
				} else {
					answer = fmt.Sprintf("%s failed — %s. Stopped without further action.", name, callErr.Error())
				}
				emit(Step{Kind: StepError, Title: name + " failed", Detail: callErr.Error()})
				emit(Step{Kind: StepAnswer, Title: "Stopped", Detail: answer})
				// Pair the call with its real outcome. Left unpaired, the next
				// turn's RepairPairing fills in "(not executed)" and the model
				// reads a tool that errored as one that cannot run at all.
				messages = append(messages, llm.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Name:       name,
					Content:    "error: " + callErr.Error(),
				})
				return answer, nil
			}
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

func toolNameSet(names []string) map[string]struct{} {
	if len(names) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			set[name] = struct{}{}
		}
	}
	return set
}

func historicalAgentToolNames(prior []llm.Message, base []llm.Tool) map[string]struct{} {
	baseNames := toolNameSet(toolNames(base))
	remembered := make(map[string]struct{})
	for _, msg := range prior {
		if msg.Role != "tool" {
			continue
		}
		name := strings.TrimSpace(msg.Name)
		if name == "" {
			continue
		}
		if _, isBaseTool := baseNames[name]; isBaseTool {
			continue
		}
		remembered[name] = struct{}{}
	}
	if len(remembered) == 0 {
		return nil
	}
	return remembered
}

// truncate shortens s for step/detail display (the llm package keeps its own copy).
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func runlogID(trace *runlog.Session) string {
	if trace == nil {
		return ""
	}
	return trace.RunID()
}

func resolveChainRPC(cfg Config) string {
	if u := strings.TrimSpace(cfg.ChainRPCURL); u != "" {
		return u
	}
	return chainrpc.URLForChain(cfg.ChainID)
}
