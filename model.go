package drel

import "time"

type Model[K comparable] struct {
	id        K
	createdAt time.Time
	updatedAt time.Time
	events    []any
}

func (m Model[K]) ID() K                { return m.id }
func (m Model[K]) CreatedAt() time.Time { return m.createdAt }
func (m Model[K]) UpdatedAt() time.Time { return m.updatedAt }

// ScanPtrs returns pointers to the base model fields for use in row scanning.
// Generated code calls this to obtain scan destinations for id, createdAt,
// and updatedAt without directly accessing unexported fields.
func (m *Model[K]) ScanPtrs() (*K, *time.Time, *time.Time) {
	return &m.id, &m.createdAt, &m.updatedAt
}

// SetID sets the primary key. Used by generated key strategies (app-assigned
// keys) and by application code that supplies its own keys.
func (m *Model[K]) SetID(id K) { m.id = id }

// RecordEvent stages a domain event on the entity. Recorded events are
// dispatched (to after-commit handlers and any outbox) when the entity is
// persisted by SaveChanges — i.e. when it is inserted, updated, or deleted in
// that unit of work. Recording an event on an entity that is otherwise
// unchanged (no field mutation, not added or removed) does not, on its own,
// cause a flush, so its events are not dispatched until the entity is also
// mutated. Record events alongside the mutation that produced them.
func (m *Model[K]) RecordEvent(event any) {
	m.events = append(m.events, event)
}

func (m *Model[K]) PendingEvents() []any {
	return m.events
}

func (m *Model[K]) ClearEvents() {
	m.events = nil
}

type SoftDelete struct {
	deletedAt *time.Time
}

func (s SoftDelete) DeletedAt() *time.Time      { return s.deletedAt }
func (s *SoftDelete) DeletedAtPtr() **time.Time { return &s.deletedAt }
func (s SoftDelete) IsDeleted() bool            { return s.deletedAt != nil }

type Versioned struct {
	version int
}

func (v Versioned) Version() int      { return v.version }
func (v *Versioned) VersionPtr() *int { return &v.version }

type Audit struct {
	createdBy string
	updatedBy string
}

func (a Audit) CreatedBy() string              { return a.createdBy }
func (a Audit) UpdatedBy() string              { return a.updatedBy }
func (a *Audit) AuditPtrs() (*string, *string) { return &a.createdBy, &a.updatedBy }
