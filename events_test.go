package drel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testEvent struct{ Name string }

func TestModel_RecordEvent(t *testing.T) {
	type testModel struct {
		Model[int]
	}
	m := &testModel{}

	m.RecordEvent(testEvent{Name: "created"})
	m.RecordEvent(testEvent{Name: "updated"})

	events := m.PendingEvents()
	assert.Len(t, events, 2)
	assert.Equal(t, testEvent{Name: "created"}, events[0])
	assert.Equal(t, testEvent{Name: "updated"}, events[1])
}

func TestModel_ClearEvents(t *testing.T) {
	type testModel struct {
		Model[int]
	}
	m := &testModel{}
	m.RecordEvent(testEvent{Name: "created"})

	m.ClearEvents()

	assert.Empty(t, m.PendingEvents())
}

func TestModel_PendingEvents_EmptyByDefault(t *testing.T) {
	type testModel struct {
		Model[int]
	}
	m := &testModel{}

	assert.Empty(t, m.PendingEvents())
}

func TestEngine_OnAfterCommit_RegistersHook(t *testing.T) {
	e := &Engine{}
	e.OnAfterCommit(func(ctx context.Context, events []any) {})

	assert.Len(t, e.afterCommitHooks, 1)
}

func TestEngine_OnBeforeCommit_RegistersHook(t *testing.T) {
	e := &Engine{}
	e.OnBeforeCommit(func(ctx context.Context, tx *Tx, events []any) error { return nil })

	assert.Len(t, e.beforeCommitHooks, 1)
}

func TestCollectPendingEvents_FromTrackedEntities(t *testing.T) {
	type user struct {
		Model[int]
		name string
	}
	u := &user{name: "Alice"}
	u.RecordEvent(testEvent{Name: "created"})
	u.RecordEvent(testEvent{Name: "updated"})

	tracker := newChangeTracker()
	meta := &ModelMetaBase{Table: "users"}
	tracker.MarkAdded(u, meta)

	events := collectPendingEvents(tracker)
	assert.Len(t, events, 2)
	assert.Equal(t, testEvent{Name: "created"}, events[0])
	assert.Equal(t, testEvent{Name: "updated"}, events[1])
	assert.Empty(t, u.PendingEvents())
}

func TestCollectPendingEvents_SkipsNonEventRecorders(t *testing.T) {
	type plain struct{ Name string }
	p := &plain{Name: "test"}

	tracker := newChangeTracker()
	meta := &ModelMetaBase{Table: "plains"}
	tracker.MarkAdded(p, meta)

	events := collectPendingEvents(tracker)
	assert.Empty(t, events)
}

func TestCollectPendingEvents_MultipleEntities(t *testing.T) {
	type user struct {
		Model[int]
		name string
	}
	u1 := &user{name: "Alice"}
	u1.RecordEvent(testEvent{Name: "alice"})
	u2 := &user{name: "Bob"}
	u2.RecordEvent(testEvent{Name: "bob"})

	tracker := newChangeTracker()
	meta := &ModelMetaBase{Table: "users"}
	tracker.MarkAdded(u1, meta)
	tracker.MarkAdded(u2, meta)

	events := collectPendingEvents(tracker)
	assert.Len(t, events, 2)
	assert.Equal(t, testEvent{Name: "alice"}, events[0])
	assert.Equal(t, testEvent{Name: "bob"}, events[1])
}
