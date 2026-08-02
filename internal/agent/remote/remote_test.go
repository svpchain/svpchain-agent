package remote

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsConnectionError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{context.Canceled, false},
		{context.DeadlineExceeded, false},
		{fmt.Errorf("connection closed: calling %q", "tools/list"), true},
		{errors.New("client is closing: hanging GET: failed to reconnect: Bad Gateway"), true},
		{errors.New("permission denied"), false},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, isConnectionError(tc.err), tc.err)
	}
}
