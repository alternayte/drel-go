package drel

import (
	"errors"
	"fmt"
)

type entityState int

const (
	StateUnchanged entityState = iota
	StateAdded
	StateModified
	StateDeleted
)

// ErrEntityNotTracked is returned when an operation requires a tracked entity but none is found.
var ErrEntityNotTracked = errors.New("drel: entity is not tracked")

type modelMetaBase struct {
	Table         string
	Columns       []string
	PKColumn      string
	Snapshot      func(entity any) any
	Diff          func(entity any, snapshot any) []FieldChange
	PKValue       func(entity any) any
	InsertColumns func(entity any) ([]string, []any)
	ScanRow       func(Row) (any, error)
}

type trackedEntity struct {
	entity   any
	state    entityState
	snapshot any
	meta     *modelMetaBase
}

type changeTracker struct {
	entities []*trackedEntity
	index    map[any]*trackedEntity
}

func newChangeTracker() *changeTracker {
	return &changeTracker{
		index: make(map[any]*trackedEntity),
	}
}

func (ct *changeTracker) Track(entity any, snapshot any, meta *modelMetaBase) {
	if _, exists := ct.index[entity]; exists {
		return
	}
	te := &trackedEntity{
		entity:   entity,
		state:    StateUnchanged,
		snapshot: snapshot,
		meta:     meta,
	}
	ct.entities = append(ct.entities, te)
	ct.index[entity] = te
}

func (ct *changeTracker) MarkAdded(entity any, meta *modelMetaBase) {
	if _, exists := ct.index[entity]; exists {
		return
	}
	te := &trackedEntity{
		entity: entity,
		state:  StateAdded,
		meta:   meta,
	}
	ct.entities = append(ct.entities, te)
	ct.index[entity] = te
}

func (ct *changeTracker) MarkDeleted(entity any) error {
	te, exists := ct.index[entity]
	if !exists {
		return fmt.Errorf("%w: cannot remove an entity that is not tracked", ErrEntityNotTracked)
	}
	te.state = StateDeleted
	return nil
}

func (ct *changeTracker) DetectChanges() {
	for _, te := range ct.entities {
		if te.state != StateUnchanged {
			continue
		}
		changes := te.meta.Diff(te.entity, te.snapshot)
		if len(changes) > 0 {
			te.state = StateModified
		}
	}
}

type pendingChanges struct {
	Added    []*trackedEntity
	Modified []*trackedEntity
	Deleted  []*trackedEntity
}

func (ct *changeTracker) GetPendingChanges() pendingChanges {
	var pc pendingChanges
	for _, te := range ct.entities {
		switch te.state {
		case StateAdded:
			pc.Added = append(pc.Added, te)
		case StateModified:
			pc.Modified = append(pc.Modified, te)
		case StateDeleted:
			pc.Deleted = append(pc.Deleted, te)
		}
	}
	return pc
}

func (ct *changeTracker) PostFlush() {
	surviving := ct.entities[:0]
	for _, te := range ct.entities {
		switch te.state {
		case StateAdded:
			te.snapshot = te.meta.Snapshot(te.entity)
			te.state = StateUnchanged
			surviving = append(surviving, te)
		case StateModified:
			te.snapshot = te.meta.Snapshot(te.entity)
			te.state = StateUnchanged
			surviving = append(surviving, te)
		case StateDeleted:
			delete(ct.index, te.entity)
		case StateUnchanged:
			surviving = append(surviving, te)
		}
	}
	ct.entities = surviving
}
