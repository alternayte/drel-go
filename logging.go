package drel

import (
	"context"
	"time"
)

// QueryEvent contains information about an executed query.
type QueryEvent struct {
	SQL      string
	Args     []any
	Duration time.Duration
	Err      error
}

// QueryHook is called after every query execution.
type QueryHook func(ctx context.Context, event QueryEvent)

// OnQuery registers a hook that is called after every query execution.
func (e *Engine) OnQuery(hook QueryHook) {
	e.hookMu.Lock()
	e.queryHooks = append(e.queryHooks, hook)
	e.hookMu.Unlock()
}

// snapshotBeforeCommitHooks returns a copy of the registered before-commit hooks
// taken under the read lock, safe to range without holding the lock.
func (e *Engine) snapshotBeforeCommitHooks() []BeforeCommitHook {
	e.hookMu.RLock()
	defer e.hookMu.RUnlock()
	if len(e.beforeCommitHooks) == 0 {
		return nil
	}
	return append([]BeforeCommitHook(nil), e.beforeCommitHooks...)
}

// snapshotAfterCommitHooks returns a copy of the registered after-commit hooks.
func (e *Engine) snapshotAfterCommitHooks() []AfterCommitHook {
	e.hookMu.RLock()
	defer e.hookMu.RUnlock()
	if len(e.afterCommitHooks) == 0 {
		return nil
	}
	return append([]AfterCommitHook(nil), e.afterCommitHooks...)
}

func (e *Engine) notifyQueryHooks(ctx context.Context, sql string, args []any, dur time.Duration, err error) {
	e.hookMu.RLock()
	hooks := e.queryHooks
	if len(hooks) == 0 {
		e.hookMu.RUnlock()
		return
	}
	snapshot := append([]QueryHook(nil), hooks...)
	e.hookMu.RUnlock()
	event := QueryEvent{SQL: sql, Args: args, Duration: dur, Err: err}
	for _, hook := range snapshot {
		hook(ctx, event)
	}
}
