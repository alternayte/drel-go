package models

import "github.com/alternayte/drel"

// Note is a simple SQLite-backed model. The db tag options declare an index on
// `category` and a unique index on `slug`, which codegen turns into CREATE INDEX
// DDL in the generated migration.
type Note struct {
	drel.Model[int]
	Slug     string `db:"slug,unique"`
	Title    string `db:"title"`
	Category string `db:"category,index"`
	Pinned   bool   `db:"pinned"`
}

func NewNote(slug, title, category string) *Note {
	return &Note{Slug: slug, Title: title, Category: category}
}

func (n *Note) Pin()   { n.Pinned = true }
func (n *Note) Unpin() { n.Pinned = false }
