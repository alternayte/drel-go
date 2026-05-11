package drel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIncludeSpec_Unscoped(t *testing.T) {
	rel := &RelationInfo{Name: "books", Type: HasMany}
	spec := NewIncludeSpec(rel)

	assert.False(t, spec.unscoped)

	unscoped := spec.Unscoped()
	assert.True(t, unscoped.unscoped)
	assert.False(t, spec.unscoped) // original unchanged
}

func TestToMetaBase_CopiesFilters(t *testing.T) {
	meta := &ModelMeta[testModel]{
		Table:    "test",
		Columns:  []string{"id"},
		PKColumn: "id",
		Scan:     func(Row) (*testModel, error) { return nil, nil },
		Snapshot: func(*testModel) any { return nil },
		Diff:     func(*testModel, any) []FieldChange { return nil },
		PKValue:  func(*testModel) any { return nil },
		InsertColumns: func(*testModel) ([]string, []any) { return nil, nil },
		Filters: []NamedFilter{SoftDeleteFilter},
	}
	base := ToMetaBase(meta)
	assert.Len(t, base.Filters, 1)
	assert.Equal(t, "soft_delete", base.Filters[0].Name)
}

type testModel struct{}
