package drel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testEntity struct {
	Name string
	Age  int
}

type testSnapshot struct {
	Name string
	Age  int
}

var testMeta = &ModelMetaBase{
	Table:    "test_entities",
	Columns:  []string{"id", "name", "age"},
	PKColumn: "id",
	Snapshot: func(entity any) any {
		e := entity.(*testEntity)
		return testSnapshot{Name: e.Name, Age: e.Age}
	},
	Diff: func(entity any, snapshot any) []FieldChange {
		e := entity.(*testEntity)
		s := snapshot.(testSnapshot)
		var changes []FieldChange
		if e.Name != s.Name {
			changes = append(changes, FieldChange{Column: "name", Value: e.Name})
		}
		if e.Age != s.Age {
			changes = append(changes, FieldChange{Column: "age", Value: e.Age})
		}
		return changes
	},
	PKValue: func(entity any) any {
		return 1
	},
	InsertColumns: func(entity any) ([]string, []any) {
		e := entity.(*testEntity)
		return []string{"name", "age"}, []any{e.Name, e.Age}
	},
}

func TestTracker_TrackSetsUnchanged(t *testing.T) {
	ct := newChangeTracker()
	e := &testEntity{Name: "Alice", Age: 30}
	snap := testSnapshot{Name: "Alice", Age: 30}
	ct.Track(e, snap, testMeta)
	te := ct.index[e]
	require.NotNil(t, te)
	assert.Equal(t, StateUnchanged, te.state)
}

func TestTracker_AddSetsAdded(t *testing.T) {
	ct := newChangeTracker()
	e := &testEntity{Name: "Bob", Age: 25}
	ct.MarkAdded(e, testMeta)
	te := ct.index[e]
	require.NotNil(t, te)
	assert.Equal(t, StateAdded, te.state)
}

func TestTracker_RemoveTrackedSetsDeleted(t *testing.T) {
	ct := newChangeTracker()
	e := &testEntity{Name: "Carol", Age: 40}
	snap := testSnapshot{Name: "Carol", Age: 40}
	ct.Track(e, snap, testMeta)
	err := ct.MarkDeleted(e)
	require.NoError(t, err)
	assert.Equal(t, StateDeleted, ct.index[e].state)
}

func TestTracker_RemoveUntrackedReturnsError(t *testing.T) {
	ct := newChangeTracker()
	e := &testEntity{Name: "Dave", Age: 35}
	err := ct.MarkDeleted(e)
	assert.ErrorIs(t, err, ErrEntityNotTracked)
}

func TestTracker_DetectChangesModified(t *testing.T) {
	ct := newChangeTracker()
	e := &testEntity{Name: "Eve", Age: 28}
	snap := testSnapshot{Name: "Eve", Age: 28}
	ct.Track(e, snap, testMeta)
	e.Name = "Eve Updated"
	ct.DetectChanges()
	assert.Equal(t, StateModified, ct.index[e].state)
}

func TestTracker_DetectChangesUnmodifiedStaysUnchanged(t *testing.T) {
	ct := newChangeTracker()
	e := &testEntity{Name: "Frank", Age: 50}
	snap := testSnapshot{Name: "Frank", Age: 50}
	ct.Track(e, snap, testMeta)
	ct.DetectChanges()
	assert.Equal(t, StateUnchanged, ct.index[e].state)
}

func TestTracker_TrackSameEntityTwiceNoDuplicate(t *testing.T) {
	ct := newChangeTracker()
	e := &testEntity{Name: "Grace", Age: 22}
	snap := testSnapshot{Name: "Grace", Age: 22}
	ct.Track(e, snap, testMeta)
	ct.Track(e, snap, testMeta)
	assert.Len(t, ct.entities, 1)
}

func TestTracker_GetPendingChangesGroupsByState(t *testing.T) {
	ct := newChangeTracker()
	added := &testEntity{Name: "New", Age: 1}
	unchanged := &testEntity{Name: "Same", Age: 2}
	modified := &testEntity{Name: "Changed", Age: 3}
	deleted := &testEntity{Name: "Gone", Age: 4}
	ct.MarkAdded(added, testMeta)
	ct.Track(unchanged, testSnapshot{Name: "Same", Age: 2}, testMeta)
	ct.Track(modified, testSnapshot{Name: "Original", Age: 3}, testMeta)
	ct.Track(deleted, testSnapshot{Name: "Gone", Age: 4}, testMeta)
	ct.DetectChanges()
	ct.MarkDeleted(deleted)
	pc := ct.GetPendingChanges()
	assert.Len(t, pc.Added, 1)
	assert.Len(t, pc.Modified, 1)
	assert.Len(t, pc.Deleted, 1)
}

func TestTracker_PostFlushResetsStates(t *testing.T) {
	ct := newChangeTracker()
	added := &testEntity{Name: "New", Age: 1}
	modified := &testEntity{Name: "Changed", Age: 3}
	deleted := &testEntity{Name: "Gone", Age: 4}
	ct.MarkAdded(added, testMeta)
	ct.Track(modified, testSnapshot{Name: "Original", Age: 3}, testMeta)
	ct.Track(deleted, testSnapshot{Name: "Gone", Age: 4}, testMeta)
	ct.DetectChanges()
	ct.MarkDeleted(deleted)
	ct.PostFlush()
	assert.Equal(t, StateUnchanged, ct.index[added].state)
	assert.Equal(t, StateUnchanged, ct.index[modified].state)
	_, deletedExists := ct.index[deleted]
	assert.False(t, deletedExists)
	assert.Len(t, ct.entities, 2)
}

func TestTracker_DiffNoChangesEmptySlice(t *testing.T) {
	e := &testEntity{Name: "Alice", Age: 30}
	snap := testSnapshot{Name: "Alice", Age: 30}
	changes := testMeta.Diff(e, snap)
	assert.Empty(t, changes)
}

func TestTracker_DiffSingleFieldChange(t *testing.T) {
	e := &testEntity{Name: "Bob", Age: 30}
	snap := testSnapshot{Name: "Bob", Age: 25}
	changes := testMeta.Diff(e, snap)
	require.Len(t, changes, 1)
	assert.Equal(t, "age", changes[0].Column)
	assert.Equal(t, 30, changes[0].Value)
}

func TestTracker_DiffMultipleFieldChanges(t *testing.T) {
	e := &testEntity{Name: "Carol", Age: 40}
	snap := testSnapshot{Name: "Carol Original", Age: 35}
	changes := testMeta.Diff(e, snap)
	assert.Len(t, changes, 2)
}

func TestMarkAdded_StampsAppAssignedKey(t *testing.T) {
	type widget struct{ Model[string] }
	meta := &ModelMetaBase{
		Table:       "stamp_widgets",
		KeyStrategy: KeyAppAssigned,
		GenerateKey: func() any { return "stamped" },
		SetKey:      func(e any, k any) { e.(*widget).SetID(k.(string)) },
		KeyIsZero:   func(e any) bool { return e.(*widget).ID() == "" },
	}

	ct := newChangeTracker()
	w := &widget{}
	ct.MarkAdded(w, meta)

	if w.ID() != "stamped" {
		t.Fatalf("expected stamped id, got %q", w.ID())
	}
}

func TestMarkAdded_DoesNotRestampNonZeroKey(t *testing.T) {
	type widget struct{ Model[string] }
	meta := &ModelMetaBase{
		Table:       "stamp_widgets2",
		KeyStrategy: KeyAppAssigned,
		GenerateKey: func() any { return "stamped" },
		SetKey:      func(e any, k any) { e.(*widget).SetID(k.(string)) },
		KeyIsZero:   func(e any) bool { return e.(*widget).ID() == "" },
	}

	ct := newChangeTracker()
	w := &widget{}
	w.SetID("preset")
	ct.MarkAdded(w, meta)

	if w.ID() != "preset" {
		t.Fatalf("expected preset id preserved, got %q", w.ID())
	}
}

func TestStampKey_RegistryOverridesMetaGenerator(t *testing.T) {
	type widget struct{ Model[string] }
	const table = "override_widgets"
	defer clearKeyConfig(table)
	meta := &ModelMetaBase{
		Table:       table,
		KeyStrategy: KeyAppAssigned,
		GenerateKey: func() any { return "meta-default" },
		SetKey:      func(e any, k any) { e.(*widget).SetID(k.(string)) },
		KeyIsZero:   func(e any) bool { return e.(*widget).ID() == "" },
	}
	setKeyConfig(table, keyConfig{Strategy: KeyAppAssigned, Generate: func() any { return "registry-override" }})

	ct := newChangeTracker()
	w := &widget{}
	ct.MarkAdded(w, meta)
	if w.ID() != "registry-override" {
		t.Fatalf("expected registry override to win, got %q", w.ID())
	}
}

func TestTracker_AddThenRemoveCancels(t *testing.T) {
	ct := newChangeTracker()
	e := &testEntity{Name: "Ghost", Age: 1}
	ct.MarkAdded(e, testMeta)

	require.NoError(t, ct.MarkDeleted(e))

	// The add/remove must cancel: the entity is no longer tracked and produces
	// no pending changes (so no stray DELETE against a zero PK is emitted).
	_, exists := ct.index[e]
	assert.False(t, exists, "Add then Remove must detach the entity entirely")
	assert.Len(t, ct.entities, 0)
	pc := ct.GetPendingChanges()
	assert.Empty(t, pc.Added)
	assert.Empty(t, pc.Deleted)
}

func TestTracker_AddRemoveAddReAdds(t *testing.T) {
	ct := newChangeTracker()
	e := &testEntity{Name: "Phoenix", Age: 2}
	ct.MarkAdded(e, testMeta)
	require.NoError(t, ct.MarkDeleted(e))

	// Re-adding after a cancel must track it again as Added.
	ct.MarkAdded(e, testMeta)
	te, exists := ct.index[e]
	require.True(t, exists)
	assert.Equal(t, StateAdded, te.state)
	pc := ct.GetPendingChanges()
	assert.Len(t, pc.Added, 1)
}

func TestTracker_RemoveLoadedStillDeletes(t *testing.T) {
	// Removing a loaded (tracked-from-query) entity must still mark it deleted.
	ct := newChangeTracker()
	e := &testEntity{Name: "Real", Age: 3}
	ct.Track(e, testSnapshot{Name: "Real", Age: 3}, testMeta)
	require.NoError(t, ct.MarkDeleted(e))
	assert.Equal(t, StateDeleted, ct.index[e].state)
}

func TestTracker_ForceUpdateClearedAfterFinalize(t *testing.T) {
	ct := newChangeTracker()
	e := &testEntity{Name: "Attached", Age: 10}
	// Attach as Modified sets forceUpdate (full-column UPDATE on first flush).
	ct.Attach(e, StateModified, testMeta)
	te := ct.index[e]
	require.NotNil(t, te)
	require.True(t, te.forceUpdate, "Attach(StateModified) must set forceUpdate")

	// Finalizing after a successful commit must clear forceUpdate so a later
	// mutation diffs only the changed columns instead of writing every column.
	ct.PostFlush()

	assert.False(t, te.forceUpdate,
		"forceUpdate must be cleared after finalize so later mutations diff")
	assert.Equal(t, StateUnchanged, te.state)
}
