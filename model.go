package drel

import "time"

type Model[K comparable] struct {
	id        K
	createdAt time.Time
	updatedAt time.Time
}

func (m Model[K]) ID() K               { return m.id }
func (m Model[K]) CreatedAt() time.Time { return m.createdAt }
func (m Model[K]) UpdatedAt() time.Time { return m.updatedAt }

// ScanPtrs returns pointers to the base model fields for use in row scanning.
// Generated code calls this to obtain scan destinations for id, createdAt,
// and updatedAt without directly accessing unexported fields.
func (m *Model[K]) ScanPtrs() (*K, *time.Time, *time.Time) {
	return &m.id, &m.createdAt, &m.updatedAt
}

type SoftDelete struct {
	deletedAt *time.Time
}

func (s SoftDelete) DeletedAt() *time.Time    { return s.deletedAt }
func (s *SoftDelete) DeletedAtPtr() **time.Time { return &s.deletedAt }

type Versioned struct {
	version int
}

func (v Versioned) Version() int    { return v.version }
func (v *Versioned) VersionPtr() *int { return &v.version }

type Audit struct {
	createdBy string
	updatedBy string
}

func (a Audit) CreatedBy() string { return a.createdBy }
func (a Audit) UpdatedBy() string { return a.updatedBy }
