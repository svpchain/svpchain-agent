package agent

import (
	"context"
	"sync"
)

// RemotePool keeps live remote MCP sessions keyed by chain id + remote URL.
type RemotePool struct {
	mu      sync.Mutex
	entries map[string]*remotePoolEntry
}

type remotePoolEntry struct {
	chainID   string
	remoteURL string
	owner     string
	remote    *RemoteClient
}

var defaultRemotePool = NewRemotePool()

// NewRemotePool returns an empty remote session pool.
func NewRemotePool() *RemotePool {
	return &RemotePool{entries: make(map[string]*remotePoolEntry)}
}

func remotePoolKey(chainID, remoteURL string) string {
	return chainID + "\x00" + remoteURL
}

// Acquire returns a connected and authenticated remote client, creating or reusing
// a pooled session. Progress steps are emitted only when a connection or re-auth
// actually runs.
func (p *RemotePool) Acquire(ctx context.Context, chainID, remoteURL, owner string, signChallenge func(challenge string) (string, error), emit func(Step)) (*RemoteClient, error) {
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
			remote:    NewRemoteClient(remoteURL),
		}
		p.entries[key] = ent
	}
	remote := ent.remote
	p.mu.Unlock()

	if !remote.IsConnected() {
		emit(Step{Kind: StepThink, Title: "Connecting to remote MCP…", Detail: remoteURL})
		if err := remote.Connect(ctx); err != nil {
			return nil, err
		}
	}

	if !remote.BearerValid() {
		emit(Step{Kind: StepAuth, Title: "Authenticating with remote MCP…"})
		if err := remote.EnsureAuth(ctx, owner, signChallenge); err != nil {
			emit(Step{Kind: StepError, Title: "Authentication failed", Detail: err.Error()})
			return nil, err
		}
		emit(Step{Kind: StepAuth, Title: "Authenticated", Detail: owner})
	}

	return remote, nil
}

// Shutdown closes all pooled remote MCP sessions.
func (p *RemotePool) Shutdown() {
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

// ShutdownRemotePool closes the process-wide default remote MCP pool.
func ShutdownRemotePool() {
	defaultRemotePool.Shutdown()
}

// SetRemotePoolForTest replaces the process-wide pool (tests only).
func SetRemotePoolForTest(p *RemotePool) {
	if p == nil {
		p = NewRemotePool()
	}
	defaultRemotePool = p
}

func acquireRemote(ctx context.Context, chainID, remoteURL, owner string, signChallenge func(challenge string) (string, error), emit func(Step)) (*RemoteClient, error) {
	return defaultRemotePool.Acquire(ctx, chainID, remoteURL, owner, signChallenge, emit)
}
