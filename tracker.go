package drel

import (
	"errors"
	"fmt"
)

// EntityState describes a tracked entity's lifecycle state within a Tx.
type EntityState int

const (
	// StateUnchanged marks an entity with no pending changes.
	StateUnchanged EntityState = iota
	// StateAdded marks an entity to be INSERTed on the next flush.
	StateAdded
	// StateModified marks an entity to be UPDATEd on the next flush.
	StateModified
	// StateDeleted marks an entity to be deleted on the next flush.
	StateDeleted
)

// ErrEntityNotTracked is returned when an operation requires a tracked entity but none is found.
var ErrEntityNotTracked = errors.New("drel: entity is not tracked")

// ModelMetaBase is the type-erased version of ModelMeta[T], used internally
// by the change tracker and relationship loading infrastructure.
type ModelMetaBase struct {
	Table          string
	Columns        []string
	PKColumn       string
	Snapshot       func(entity any) any
	Diff           func(entity any, snapshot any) []FieldChange
	PKValue        func(entity any) any
	InsertColumns  func(entity any) ([]string, []any)
	ScanRow        func(Row) (any, error)
	ScanReturning  func(entity any, row Row) error
	ScanGenerated  func(entity any, row Row) error
	KeyStrategy    KeyStrategy
	GenerateKey    func() any
	SetKey         func(entity any, key any)
	KeyIsZero      func(entity any) bool
	ColumnValue    func(entity any, colIdx int) any
	HasSoftDelete  bool
	HasVersioned   bool
	HasAudit       bool
	VersionValue   func(entity any) int
	SetVersion     func(entity any, v int)
	AuditSetCreate func(entity any, actor string)
	AuditSetUpdate func(entity any, actor string)
	Filters        []NamedFilter
}

type trackedEntity struct {
	entity      any
	state       EntityState
	snapshot    any
	meta        *ModelMetaBase
	hardDelete  bool
	forceUpdate bool // attached as Modified: UPDATE all columns rather than diffing
	loaded      bool // tracked from a query (not Added) — eligible for unused-tracking hint
	everDirty   bool // ever transitioned to Modified/Deleted during this transaction
	flushed     bool // SQL emitted this flush; skip on re-flush, finalized by PostCommit
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

func (ct *changeTracker) Track(entity any, snapshot any, meta *ModelMetaBase) {
	if _, exists := ct.index[entity]; exists {
		return
	}
	te := &trackedEntity{
		entity:   entity,
		state:    StateUnchanged,
		snapshot: snapshot,
		meta:     meta,
		loaded:   true,
	}
	ct.entities = append(ct.entities, te)
	ct.index[entity] = te
}

func (ct *changeTracker) MarkAdded(entity any, meta *ModelMetaBase) {
	if _, exists := ct.index[entity]; exists {
		return
	}
	stampKey(entity, meta)
	te := &trackedEntity{
		entity: entity,
		state:  StateAdded,
		meta:   meta,
	}
	ct.entities = append(ct.entities, te)
	ct.index[entity] = te
}

// stampKey assigns an app-assigned primary key before insert when one is needed
// and not already set. The per-table registry (drel.SetKeyStrategy /
// SetKeyGenerator) takes precedence over the meta's codegen defaults.
func stampKey(entity any, meta *ModelMetaBase) {
	strategy := meta.KeyStrategy
	gen := meta.GenerateKey
	if cfg, ok := keyConfigFor(meta.Table); ok {
		strategy = cfg.Strategy
		gen = cfg.Generate
	}
	if strategy != KeyAppAssigned || gen == nil {
		return
	}
	if meta.SetKey == nil || meta.KeyIsZero == nil {
		return
	}
	if meta.KeyIsZero(entity) {
		meta.SetKey(entity, gen())
	}
}

// Attach begins tracking an externally-constructed entity in the given state.
// StateModified flags the entity for a full-column UPDATE on the next flush
// (no snapshot diff, since the prior values are unknown).
func (ct *changeTracker) Attach(entity any, state EntityState, meta *ModelMetaBase) {
	if te, exists := ct.index[entity]; exists {
		te.state = state
		te.meta = meta
		te.forceUpdate = state == StateModified
		return
	}
	te := &trackedEntity{
		entity:      entity,
		state:       state,
		meta:        meta,
		forceUpdate: state == StateModified,
		loaded:      state == StateUnchanged,
	}
	if state == StateUnchanged && meta.Snapshot != nil {
		te.snapshot = meta.Snapshot(entity)
	}
	ct.entities = append(ct.entities, te)
	ct.index[entity] = te
}

// Detach stops tracking an entity. Subsequent mutations to it are not flushed.
func (ct *changeTracker) Detach(entity any) {
	te, exists := ct.index[entity]
	if !exists {
		return
	}
	delete(ct.index, entity)
	for i, e := range ct.entities {
		if e == te {
			ct.entities = append(ct.entities[:i], ct.entities[i+1:]...)
			break
		}
	}
}

func (ct *changeTracker) MarkDeleted(entity any) error {
	te, exists := ct.index[entity]
	if !exists {
		return fmt.Errorf("%w: cannot remove an entity that is not tracked", ErrEntityNotTracked)
	}
	// Add-then-Remove BEFORE any flush cancels out (EF Core semantics): detach so
	// no SQL is emitted. But once the insert has been flushed to the DB in this
	// transaction (e.g. a mid-tx SaveChanges), it cannot be cancelled — fall
	// through to emit a delete instead.
	if te.state == StateAdded && !te.flushed {
		ct.Detach(entity)
		return nil
	}
	te.state = StateDeleted
	te.everDirty = true
	// Clear the stale flush marker so the new delete is picked up on the next
	// GetPendingChanges / flush (otherwise GetPendingChanges skips flushed entities).
	te.flushed = false
	return nil
}

// countUnusedTracked returns the number of entities loaded via a tracked query
// that were never modified or deleted — candidates for AsNoTracking.
func (ct *changeTracker) countUnusedTracked() int {
	n := 0
	for _, te := range ct.entities {
		if te.loaded && !te.everDirty {
			n++
		}
	}
	return n
}

func (ct *changeTracker) MarkHardDeleted(entity any) error {
	te, exists := ct.index[entity]
	if !exists {
		return fmt.Errorf("%w: cannot remove an entity that is not tracked", ErrEntityNotTracked)
	}
	te.state = StateDeleted
	te.hardDelete = true
	te.everDirty = true
	// Clear the stale flush marker so the delete is picked up on the next flush
	// even when the entity's prior state (e.g. StateAdded) was already flushed.
	te.flushed = false
	return nil
}

func (ct *changeTracker) DetectChanges() {
	for _, te := range ct.entities {
		if te.state != StateUnchanged {
			continue
		}
		// Insert-only metas (no Diff/snapshot) can't be change-detected; skip.
		if te.meta == nil || te.meta.Diff == nil || te.snapshot == nil {
			continue
		}
		changes := te.meta.Diff(te.entity, te.snapshot)
		if len(changes) > 0 {
			te.state = StateModified
			te.everDirty = true
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
		if te.flushed {
			// Already emitted in an earlier flush within this same live
			// transaction; do not re-emit until PostCommit finalizes.
			continue
		}
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

// trackerState captures the change-tracker's bookkeeping so a savepoint can
// restore it on rollback. It records the entity ordering at save time and a
// copy of each tracked entity's mutable state.
type trackerState struct {
	entities []*trackedEntity
	states   map[*trackedEntity]trackedEntity
}

// save snapshots the current tracker state for later restore.
func (ct *changeTracker) save() trackerState {
	st := trackerState{
		entities: append([]*trackedEntity(nil), ct.entities...),
		states:   make(map[*trackedEntity]trackedEntity, len(ct.entities)),
	}
	for _, te := range ct.entities {
		st.states[te] = *te
	}
	return st
}

// restore reverts the tracker to a previously saved state. Entities tracked
// since the save (e.g. Adds inside a rolled-back savepoint) are dropped, and
// the state/snapshot of surviving entities is reverted.
func (ct *changeTracker) restore(st trackerState) {
	ct.entities = append([]*trackedEntity(nil), st.entities...)
	ct.index = make(map[any]*trackedEntity, len(st.entities))
	for _, te := range ct.entities {
		*te = st.states[te]
		ct.index[te.entity] = te
	}
}

// snapshotOf re-snapshots a tracked entity after flush, tolerating metas that
// have no Snapshot function (insert-only models): such entities simply carry no
// snapshot and are never diffed.
func snapshotOf(te *trackedEntity) any {
	if te.meta == nil || te.meta.Snapshot == nil {
		return nil
	}
	return te.meta.Snapshot(te.entity)
}

// resetFlushed clears the per-flush "emitted" marker on every tracked entity.
// Called on a pre-commit failure so a retry re-emits the staged changes. State
// and snapshots were never mutated by flushChanges, so clearing the marker is
// sufficient to restore the pre-flush view.
func (ct *changeTracker) resetFlushed() {
	for _, te := range ct.entities {
		te.flushed = false
	}
}

// PostCommit finalizes the tracker after a successful commit: surviving
// entities are re-snapshotted and marked Unchanged, deleted entities are
// dropped, and the per-flush forceUpdate/flushed markers are cleared. It must
// run only after dbTx.Commit returns nil; on rollback the tracker is left
// intact (use resetFlushed) so a retry re-emits.
func (ct *changeTracker) PostCommit() {
	surviving := ct.entities[:0]
	for _, te := range ct.entities {
		switch te.state {
		case StateAdded:
			te.snapshot = snapshotOf(te)
			te.state = StateUnchanged
			te.everDirty = false
			te.forceUpdate = false
			te.flushed = false
			te.loaded = true
			surviving = append(surviving, te)
		case StateModified:
			te.snapshot = snapshotOf(te)
			te.state = StateUnchanged
			te.everDirty = false
			te.forceUpdate = false
			te.flushed = false
			te.loaded = true
			surviving = append(surviving, te)
		case StateDeleted:
			delete(ct.index, te.entity)
		case StateUnchanged:
			te.flushed = false
			surviving = append(surviving, te)
		}
	}
	ct.entities = surviving
}
