package netretry

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransient(t *testing.T) {
	require.True(t, Transient(io.ErrUnexpectedEOF))
	require.True(t, Transient(errors.New("read: connection reset by peer")))
	require.False(t, Transient(context.Canceled))
	require.False(t, Transient(errors.New("HTTP 400 Bad Request")))
}

func TestDoStopsAfterSuccessfulRetry(t *testing.T) {
	calls := 0
	err := Do(context.Background(), func() error {
		calls++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestDoRetriesAtMostThreeTimes(t *testing.T) {
	calls := 0
	err := do(context.Background(), MaxRetries, 0, func() error {
		calls++
		if calls <= MaxRetries {
			return errors.New("read: connection reset by peer")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 4, calls, "one initial call plus three retries")
}
