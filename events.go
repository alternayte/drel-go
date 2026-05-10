package drel

import "context"

type EventRecorder interface {
	PendingEvents() []any
	ClearEvents()
}

type BeforeCommitHook func(ctx context.Context, tx *Tx, events []any) error

type AfterCommitHook func(ctx context.Context, events []any)
