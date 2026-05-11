package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmitDBFile_WithRelations(t *testing.T) {
	models := []ModelInfo{
		{
			Name: "User", PkgPath: "app/models/users", PkgName: "users",
			PKType: "int", TableName: "users",
			Fields: []FieldInfo{
				{Name: "name", GoType: "string", ColumnName: "name"},
				{Name: "posts", GoType: "[]*models.Post", Relation: &RelationFieldInfo{
					Type: "has_many", FK: "user_id", TargetModel: "Post",
				}},
			},
		},
		{
			Name: "Post", PkgPath: "app/models/posts", PkgName: "posts",
			PKType: "int", TableName: "posts",
			Fields: []FieldInfo{
				{Name: "title", GoType: "string", ColumnName: "title"},
				{Name: "author", GoType: "*models.User", Relation: &RelationFieldInfo{
					Type: "belongs_to", FK: "user_id", TargetModel: "User",
				}},
			},
		},
	}

	out := EmitDBFile(models, "db")

	assert.Contains(t, out, "var UserPostsRel = drel.RelationInfo{")
	assert.Contains(t, out, "drel.ToMetaBase(&posts.PostMeta)")
	assert.Contains(t, out, "p := parent.(*users.User)")
	assert.Contains(t, out, "item.(*posts.Post)")

	assert.Contains(t, out, "var PostAuthorRel = drel.RelationInfo{")
	assert.Contains(t, out, "drel.ToMetaBase(&users.UserMeta)")

	assert.Contains(t, out, "var UserIncludePosts = drel.NewIncludeSpec(&UserPostsRel)")
	assert.Contains(t, out, "var PostIncludeAuthor = drel.NewIncludeSpec(&PostAuthorRel)")
}

func TestEmitDBFile_ManyToManyRelation(t *testing.T) {
	models := []ModelInfo{
		{
			Name: "Author", PkgPath: "app/models/authors", PkgName: "authors",
			PKType: "int", TableName: "authors",
			Fields: []FieldInfo{
				{Name: "tags", GoType: "[]*models.Tag", Relation: &RelationFieldInfo{
					Type: "many_to_many", FK: "author_id", JoinTable: "author_tags",
					RefColumn: "tag_id", TargetModel: "Tag",
				}},
			},
		},
		{
			Name: "Tag", PkgPath: "app/models/tags", PkgName: "tags",
			PKType: "int", TableName: "tags",
			Fields: []FieldInfo{
				{Name: "name", GoType: "string", ColumnName: "name"},
			},
		},
	}

	out := EmitDBFile(models, "db")

	assert.Contains(t, out, "var AuthorTagsRel = drel.RelationInfo{")
	assert.Contains(t, out, "drel.ManyToMany")
	assert.Contains(t, out, `JoinTable:   "author_tags"`)
	assert.Contains(t, out, `RefColumn:   "tag_id"`)
	assert.Contains(t, out, "drel.ToMetaBase(&tags.TagMeta)")
	assert.Contains(t, out, "var AuthorIncludeTags = drel.NewIncludeSpec(&AuthorTagsRel)")
}
