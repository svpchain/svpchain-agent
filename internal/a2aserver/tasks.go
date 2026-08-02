package a2aserver

import (
	"context"
	"sync"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

type taskRegistry struct {
	mu    sync.Mutex
	tasks map[a2a.TaskID]context.CancelFunc
}

func newTaskRegistry() *taskRegistry {
	return &taskRegistry{tasks: make(map[a2a.TaskID]context.CancelFunc)}
}

func (r *taskRegistry) register(id a2a.TaskID, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if prev, ok := r.tasks[id]; ok {
		prev()
	}
	r.tasks[id] = cancel
}

func (r *taskRegistry) cancel(id a2a.TaskID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	cancel, ok := r.tasks[id]
	if !ok {
		return false
	}
	cancel()
	delete(r.tasks, id)
	return true
}

func (r *taskRegistry) done(id a2a.TaskID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tasks, id)
}
