package codegen

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshot_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", ".drel_snapshot.json")

	s := Schema{
		Enums: []EnumDef{{Name: "role", Values: []string{"admin", "user"}}},
		Tables: []Table{
			{
				Name: "users",
				Columns: []Column{
					{Name: "id", Type: "SERIAL PRIMARY KEY", NotNull: true, PK: true},
					{Name: "email", Type: "text", NotNull: true},
					{Name: "bio", Type: "text"},
					{Name: "role", Type: `"role"`, NotNull: true},
				},
				Indexes: []Index{
					{Name: "uq_users_email", Columns: []string{"email"}, Unique: true},
				},
			},
			{
				Name: "author_tags",
				Columns: []Column{
					{Name: "author_id", Type: "integer", NotNull: true, Ref: "authors"},
					{Name: "tag_id", Type: "integer", NotNull: true, Ref: "tags"},
				},
				PrimaryKey: []string{"author_id", "tag_id"},
			},
		},
	}

	require.NoError(t, SaveSnapshot(path, s))

	loaded, ok, err := LoadSnapshot(path)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, s, loaded)
}

func TestSnapshot_LoadAbsentFile(t *testing.T) {
	dir := t.TempDir()
	loaded, ok, err := LoadSnapshot(filepath.Join(dir, "does_not_exist.json"))
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, Schema{}, loaded)
}
