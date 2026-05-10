package drel

import (
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
