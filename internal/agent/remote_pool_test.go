package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRemotePoolReusesSession(t *testing.T) {
	t.Cleanup(func() { SetRemotePoolForTest(NewRemotePool()) })

	pool := NewRemotePool()
	SetRemotePoolForTest(pool)

	var steps []string
	emit := func(s Step) { steps = append(steps, s.Title) }

	remote := NewRemoteClient("https://example.com/mcp")
	remote.forceConnected = true
	remote.mu.Lock()
	remote.bearer = "tok"
	remote.bearerUntil = time.Now().Add(time.Hour)
	remote.mu.Unlock()

	pool.mu.Lock()
	pool.entries[remotePoolKey("svp-2517-1", "https://example.com/mcp")] = &remotePoolEntry{
		chainID:   "svp-2517-1",
		remoteURL: "https://example.com/mcp",
		owner:     "svp1abc",
		remote:    remote,
	}
	pool.mu.Unlock()

	got, err := pool.Acquire(context.Background(), "svp-2517-1", "https://example.com/mcp", "svp1abc", func(string) (string, error) {
		return "", nil
	}, emit)
	require.NoError(t, err)
	require.Same(t, remote, got)
	require.Empty(t, steps)
}

func TestRemotePoolReauthsWhenBearerExpired(t *testing.T) {
	t.Cleanup(func() { SetRemotePoolForTest(NewRemotePool()) })

	pool := NewRemotePool()
	SetRemotePoolForTest(pool)

	remote := NewRemoteClient("https://example.com/mcp")
	remote.forceConnected = true
	remote.mu.Lock()
	remote.bearer = "old"
	remote.bearerUntil = time.Now().Add(-time.Minute)
	remote.mu.Unlock()

	pool.mu.Lock()
	pool.entries[remotePoolKey("svp-2517-1", "https://example.com/mcp")] = &remotePoolEntry{
		chainID:   "svp-2517-1",
		remoteURL: "https://example.com/mcp",
		owner:     "svp1abc",
		remote:    remote,
	}
	pool.mu.Unlock()

	var steps []string
	emit := func(s Step) { steps = append(steps, s.Title) }

	_, err := pool.Acquire(context.Background(), "svp-2517-1", "https://example.com/mcp", "svp1abc", func(string) (string, error) {
		return "", nil
	}, emit)
	require.Error(t, err)
	require.Contains(t, steps, "Authenticating with remote MCP…")
}

func TestRemotePoolInvalidatesOnOwnerChange(t *testing.T) {
	t.Cleanup(func() { SetRemotePoolForTest(NewRemotePool()) })

	pool := NewRemotePool()
	old := NewRemoteClient("https://example.com/mcp")
	old.forceConnected = true

	pool.mu.Lock()
	pool.entries[remotePoolKey("svp-2517-1", "https://example.com/mcp")] = &remotePoolEntry{
		chainID:   "svp-2517-1",
		remoteURL: "https://example.com/mcp",
		owner:     "svp1old",
		remote:    old,
	}
	pool.mu.Unlock()

	emit := func(Step) {}
	got, err := pool.Acquire(context.Background(), "svp-2517-1", "https://example.com/mcp", "svp1new", func(string) (string, error) {
		return "", nil
	}, emit)
	require.Error(t, err)
	require.NotSame(t, old, got)

	pool.mu.Lock()
	ent := pool.entries[remotePoolKey("svp-2517-1", "https://example.com/mcp")]
	pool.mu.Unlock()
	require.NotNil(t, ent)
	require.Equal(t, "svp1new", ent.owner)
	require.NotSame(t, old, ent.remote)
}

func TestRemotePoolShutdown(t *testing.T) {
	pool := NewRemotePool()
	r1 := NewRemoteClient("https://a.example/mcp")
	r1.forceConnected = true
	pool.entries["k1"] = &remotePoolEntry{remote: r1}
	pool.Shutdown()
	require.Empty(t, pool.entries)
	require.False(t, r1.IsConnected())
}

func TestBearerValid(t *testing.T) {
	t.Parallel()
	r := NewRemoteClient("")
	require.False(t, r.BearerValid())

	r.mu.Lock()
	r.bearer = "x"
	r.bearerUntil = time.Now().Add(time.Hour)
	r.mu.Unlock()
	require.True(t, r.BearerValid())

	r.mu.Lock()
	r.bearerUntil = time.Now().Add(30 * time.Second)
	r.mu.Unlock()
	require.False(t, r.BearerValid())
}
