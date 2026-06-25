package a2a

import (
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/stretchr/testify/require"
)

func TestMessageText(t *testing.T) {
	t.Parallel()
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hello"), a2a.NewTextPart("world"))
	require.Equal(t, "hello\nworld", MessageText(msg))
	require.Equal(t, "", MessageText(nil))
}

func TestResultTextFromMessage(t *testing.T) {
	t.Parallel()
	msg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("done"))
	require.Equal(t, "done", ResultText(msg))
}

func TestResultTextFromTask(t *testing.T) {
	t.Parallel()
	statusMsg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("status reply"))
	task := &a2a.Task{
		Status: a2a.TaskStatus{
			State:   a2a.TaskStateCompleted,
			Message: statusMsg,
		},
		History: []*a2a.Message{
			a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hi")),
			a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("history reply")),
		},
	}
	require.Equal(t, "status reply", ResultText(task))
}
