package remote

import (
	"context"
	"sync"

	"github.com/svpchain/svpchain-agent/internal/agent/step"
)

// Pool keeps live remote MCP sessions keyed by chain id + remote URL.
type Pool struct {
	mu      sync.Mutex
	entries map[string]*remotePoolEntry
}

type remotePoolEntry struct {
	chainID   string
	remoteURL string
	owner     string
	remote    *Client
}

var defaultRemotePool = NewPool()

// NewPool returns an empty remote session pool.
func NewPool() *Pool {
	return &Pool{entries: make(map[string]*remotePoolEntry)}
}

func remotePoolKey(chainID, remoteURL string) string {
	return chainID + "\x00" + remoteURL
}

// Acquire returns a connected and authenticated remote client, creating or reusing
// a pooled session. Progress steps are emitted only when a connection or re-auth
// actually runs.
func (p *Pool) Acquire(ctx context.Context, chainID, remoteURL, owner string, signChallenge func(challenge string) (string, error), emit func(step.Step)) (*Client, error) {
	key := remotePoolKey(chainID, remoteURL)

	p.mu.Lock()
	ent, ok := p.entries[key]
	if ok && ent.owner != owner {
		_ = ent.remote.Close()
		delete(p.entries, key)
		ok = false
	}
	if !ok {
		ent = &remotePoolEntry{
			chainID:   chainID,
			remoteURL: remoteURL,
			owner:     owner,
			remote:    NewClient(remoteURL),
		}
		p.entries[key] = ent
	}
	remote := ent.remote
	p.mu.Unlock()

	if !remote.IsConnected() {
		emit(step.Step{Kind: step.Think, Title: "Connecting to remote MCP…", Detail: remoteURL})
		if err := remote.Connect(ctx); err != nil {
			return nil, err
		}
	}

	if !remote.BearerValid() {
		emit(step.Step{Kind: step.Auth, Title: "Authenticating with remote MCP…"})
		if err := remote.EnsureAuth(ctx, owner, signChallenge); err != nil {
			emit(step.Step{Kind: step.Error, Title: "Authentication failed", Detail: err.Error()})
			return nil, err
		}
		emit(step.Step{Kind: step.Auth, Title: "Authenticated", Detail: owner})
	}

	return remote, nil
}

// Shutdown closes all pooled remote MCP sessions.
func (p *Pool) Shutdown() {
	p.mu.Lock()
	entries := p.entries
	p.entries = make(map[string]*remotePoolEntry)
	p.mu.Unlock()

	for _, ent := range entries {
		if ent != nil && ent.remote != nil {
			_ = ent.remote.Close()
		}
	}
}

// Shutdown closes the process-wide default remote MCP pool.
func Shutdown() {
	defaultRemotePool.Shutdown()
}

// SetPoolForTest replaces the process-wide pool (tests only).
func SetPoolForTest(p *Pool) {
	if p == nil {
		p = NewPool()
	}
	defaultRemotePool = p
}

func Acquire(ctx context.Context, chainID, remoteURL, owner string, signChallenge func(challenge string) (string, error), emit func(step.Step)) (*Client, error) {
	return defaultRemotePool.Acquire(ctx, chainID, remoteURL, owner, signChallenge, emit)
}
