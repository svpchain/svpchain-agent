package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/svpchain/svpchain-agent/internal/agent/netretry"
)

// SendResult is the JSON-friendly outcome of SendToAgent.
type SendResult struct {
	TaskID    string `json:"task_id,omitempty"`
	ContextID string `json:"context_id,omitempty"`
	State     string `json:"state,omitempty"`
	Response  string `json:"response"`
}

// SendToAgent resolves the remote Agent Card, sends a user message, and returns the agent reply.
// The message carries no credentials: it is plain text with no authority over the caller's account.
func SendToAgent(ctx context.Context, agentURL, message string) (SendResult, error) {
	return SendTextIn(ctx, agentURL, "", message)
}

// SendTextIn is SendToAgent within an existing conversation. contextID keeps
// the exchange on one A2A context, which is where a remote agent binds
// per-conversation state — an auth bearer, most importantly. Empty starts a
// new context; the id the agent assigns comes back on the result.
func SendTextIn(ctx context.Context, agentURL, contextID, message string) (SendResult, error) {
	agentURL = strings.TrimSpace(agentURL)
	message = strings.TrimSpace(message)
	if agentURL == "" {
		return SendResult{}, fmt.Errorf("agent_url is required")
	}
	if message == "" {
		return SendResult{}, fmt.Errorf("message is required")
	}

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(message))
	msg.ContextID = strings.TrimSpace(contextID)

	result, err := send(ctx, agentURL, msg)
	if err != nil {
		return SendResult{}, err
	}

	out := SendResult{Response: ResultText(result)}
	switch v := result.(type) {
	case *a2a.Task:
		out.TaskID = string(v.ID)
		out.ContextID = v.ContextID
		out.State = string(v.Status.State)
	case *a2a.Message:
		if v.TaskID != "" {
			out.TaskID = string(v.TaskID)
		}
		if v.ContextID != "" {
			out.ContextID = v.ContextID
		}
	}
	return out, nil
}

// send resolves the remote Agent Card and delivers one message. Kept separate
// from message construction so dialing an agent stays one code path.
func send(ctx context.Context, agentURL string, msg *a2a.Message) (a2a.SendMessageResult, error) {
	var card *a2a.AgentCard
	err := netretry.Do(ctx, func() error {
		var resolveErr error
		card, resolveErr = agentcard.DefaultResolver.Resolve(ctx, agentURL)
		return resolveErr
	})
	if err != nil {
		return nil, fmt.Errorf("resolve agent card: %w", err)
	}
	client, err := a2aclient.NewFromCard(ctx, card)
	if err != nil {
		return nil, fmt.Errorf("create a2a client: %w", err)
	}
	result, err := client.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg})
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}
	return result, nil
}

// ResultText extracts human-readable text from a SendMessageResult.
func ResultText(result a2a.SendMessageResult) string {
	if result == nil {
		return ""
	}
	switch v := result.(type) {
	case *a2a.Message:
		return MessageText(v)
	case *a2a.Task:
		if v.Status.Message != nil {
			if text := MessageText(v.Status.Message); text != "" {
				return text
			}
		}
		for i := len(v.History) - 1; i >= 0; i-- {
			m := v.History[i]
			if m != nil && m.Role == a2a.MessageRoleAgent {
				if text := MessageText(m); text != "" {
					return text
				}
			}
		}
	}
	return ""
}

// SendToAgentJSON is a convenience wrapper returning JSON for agent tools.
func SendToAgentJSON(ctx context.Context, agentURL, message string) (string, error) {
	res, err := SendToAgent(ctx, agentURL, message)
	if err != nil {
		return "", err
	}
	bz, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(bz), nil
}
