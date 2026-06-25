package a2aserver

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	svpa2a "github.com/svpchain/svpchain-agent/internal/a2a"
	svpagent "github.com/svpchain/svpchain-agent/internal/agent"
)

// Executor runs svpchain agent orchestration for incoming A2A tasks.
type Executor struct {
	cfg      ServerConfig
	registry *taskRegistry
}

var _ a2asrv.AgentExecutor = (*Executor)(nil)

// NewExecutor returns an A2A AgentExecutor backed by agent.Run.
func NewExecutor(cfg ServerConfig) *Executor {
	return &Executor{
		cfg:      cfg,
		registry: newTaskRegistry(),
	}
}

func (e *Executor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if execCtx.Message == nil {
			yield(nil, errEmptyMessage)
			return
		}
		userMessage := svpa2a.MessageText(execCtx.Message)
		if userMessage == "" {
			yield(nil, errEmptyMessage)
			return
		}
		if strings.TrimSpace(e.cfg.LLM.APIKey) == "" {
			yield(nil, errNoLLMKey)
			return
		}

		if execCtx.StoredTask == nil {
			if !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
				return
			}
		}
		if !yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking, nil), nil) {
			return
		}

		runCtx, cancel := context.WithCancel(ctx)
		e.registry.register(execCtx.TaskID, cancel)
		defer e.registry.done(execCtx.TaskID)
		defer cancel()

		var artifactID a2a.ArtifactID
		emitStep := func(step svpagent.Step) {
			line := step.Title
			if step.Detail != "" {
				line = step.Title + "\n" + step.Detail
			}
			event := a2a.NewArtifactEvent(execCtx, a2a.NewTextPart(line))
			if artifactID == "" {
				artifactID = event.Artifact.ID
			} else {
				event = a2a.NewArtifactUpdateEvent(execCtx, artifactID, a2a.NewTextPart(line))
			}
			yield(event, nil)
		}

		answer, err := svpagent.Run(runCtx, svpagent.Config{
			ChainID:   e.cfg.ChainID,
			RemoteURL: e.cfg.RemoteMCPURL,
			LLM: svpagent.LLMConfig{
				APIKey:  e.cfg.LLM.APIKey,
				BaseURL: e.cfg.LLM.BaseURL,
				Model:   e.cfg.LLM.Model,
			},
			OnStep: emitStep,
		}, userMessage)
		if err != nil {
			if runCtx.Err() != nil {
				yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
				return
			}
			msg := a2a.NewMessageForTask(a2a.MessageRoleAgent, execCtx, a2a.NewTextPart(fmt.Sprintf("Error: %v", err)))
			yield(msg, nil)
			return
		}

		yield(a2a.NewMessageForTask(a2a.MessageRoleAgent, execCtx, a2a.NewTextPart(answer)), nil)
	}
}

func (e *Executor) Cancel(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if e.registry.cancel(execCtx.TaskID) {
			yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
			return
		}
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}
