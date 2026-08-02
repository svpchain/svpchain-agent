package desktop

import (
	"context"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/svpchain/svpchain-local-agent/internal/agent/delegatecall"
)

// confirmTimeout bounds how long a grant dialog may sit unanswered. A dialog
// nobody answers is a denial, never an approval.
const confirmTimeout = 2 * time.Minute

var (
	confirmMu      sync.Mutex
	confirmSeq     int
	confirmPending = map[int]chan bool{}
)

// confirmHook bridges the agent's grant requests to the frontend: emits
// "agent:confirm" with the request and blocks until ResolveConfirm answers,
// the run is cancelled, or the timeout passes.
func (a *App) confirmHook(ctx context.Context, req delegatecall.ConfirmRequest) bool {
	confirmMu.Lock()
	confirmSeq++
	id := confirmSeq
	ch := make(chan bool, 1)
	confirmPending[id] = ch
	confirmMu.Unlock()

	defer func() {
		confirmMu.Lock()
		delete(confirmPending, id)
		confirmMu.Unlock()
	}()

	wruntime.EventsEmit(a.ctx, "agent:confirm", map[string]any{
		"id":    id,
		"kind":  req.Kind,
		"title": req.Title,
		"lines": req.Lines,
	})

	select {
	case approved := <-ch:
		return approved
	case <-ctx.Done():
		return false
	case <-time.After(confirmTimeout):
		wruntime.EventsEmit(a.ctx, "agent:confirm-expired", map[string]any{"id": id})
		return false
	}
}

// ResolveConfirm answers a pending grant dialog from the frontend.
func (a *App) ResolveConfirm(id int, approved bool) {
	confirmMu.Lock()
	ch, ok := confirmPending[id]
	delete(confirmPending, id)
	confirmMu.Unlock()
	if ok {
		ch <- approved
	}
}
