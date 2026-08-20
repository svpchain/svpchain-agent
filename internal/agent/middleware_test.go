package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrapAppliesOuterFirst(t *testing.T) {
	var order []string
	h := wrap(func(context.Context, string, map[string]any) (string, error) {
		order = append(order, "route")
		return "ok", nil
	}, func(next toolFunc) toolFunc {
		return func(ctx context.Context, name string, args map[string]any) (string, error) {
			order = append(order, "guard")
			return next(ctx, name, args)
		}
	}, func(next toolFunc) toolFunc {
		return func(ctx context.Context, name string, args map[string]any) (string, error) {
			order = append(order, "writepath")
			return next(ctx, name, args)
		}
	})
	out, err := h(context.Background(), "whoami", nil)
	require.NoError(t, err)
	require.Equal(t, "ok", out)
	require.Equal(t, []string{"guard", "writepath", "route"}, order)
}
