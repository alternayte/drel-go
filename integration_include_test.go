//go:build integration

package drel_test

import (
	"context"
	"testing"
	"time"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test models ---

type Author struct {
	ID        int
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Books     []*Book
	Profile   *AuthorProfile
}

type Book struct {
	ID        int
	Title     string
	AuthorID  int
	CreatedAt time.Time
	UpdatedAt time.Time
	Author    *Author
}

type AuthorProfile struct {
	ID        int
	Bio       string
	AuthorID  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// --- ModelMeta definitions ---

var authorMeta = drel.ModelMeta[Author]{
	Table:    "authors",
	Columns:  []string{"id", "name", "created_at", "updated_at"},
	PKColumn: "id",
	Scan: func(row drel.Row) (*Author, error) {
		a := &Author{}
		err := row.Scan(&a.ID, &a.Name, &a.CreatedAt, &a.UpdatedAt)
		if err != nil {
			return nil, err
		}
		return a, nil
	},
	PKValue: func(a *Author) any {
		return a.ID
	},
	ColumnValue: func(a *Author, idx int) any {
		switch idx {
		case 0:
			return a.ID
		case 1:
			return a.Name
		case 2:
			return a.CreatedAt
		case 3:
			return a.UpdatedAt
		}
		return nil
	},
	Snapshot: func(a *Author) any {
		return a.Name
	},
	Diff: func(a *Author, snap any) []drel.FieldChange {
		if a.Name != snap.(string) {
			return []drel.FieldChange{{Column: "name", Value: a.Name}}
		}
		return nil
	},
	InsertColumns: func(a *Author) ([]string, []any) {
		return []string{"name"}, []any{a.Name}
	},
	ScanReturning: func(a *Author, row drel.Row) error {
		return row.Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	},
}

var bookMeta = drel.ModelMeta[Book]{
	Table:    "books",
	Columns:  []string{"id", "title", "author_id", "created_at", "updated_at"},
	PKColumn: "id",
	Scan: func(row drel.Row) (*Book, error) {
		b := &Book{}
		err := row.Scan(&b.ID, &b.Title, &b.AuthorID, &b.CreatedAt, &b.UpdatedAt)
		if err != nil {
			return nil, err
		}
		return b, nil
	},
	PKValue: func(b *Book) any {
		return b.ID
	},
	ColumnValue: func(b *Book, idx int) any {
		switch idx {
		case 0:
			return b.ID
		case 1:
			return b.Title
		case 2:
			return b.AuthorID
		case 3:
			return b.CreatedAt
		case 4:
			return b.UpdatedAt
		}
		return nil
	},
	Snapshot: func(b *Book) any {
		return b.Title
	},
	Diff: func(b *Book, snap any) []drel.FieldChange {
		if b.Title != snap.(string) {
			return []drel.FieldChange{{Column: "title", Value: b.Title}}
		}
		return nil
	},
	InsertColumns: func(b *Book) ([]string, []any) {
		return []string{"title", "author_id"}, []any{b.Title, b.AuthorID}
	},
	ScanReturning: func(b *Book, row drel.Row) error {
		return row.Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
	},
}

var profileMeta = drel.ModelMeta[AuthorProfile]{
	Table:    "author_profiles",
	Columns:  []string{"id", "bio", "author_id", "created_at", "updated_at"},
	PKColumn: "id",
	Scan: func(row drel.Row) (*AuthorProfile, error) {
		p := &AuthorProfile{}
		err := row.Scan(&p.ID, &p.Bio, &p.AuthorID, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			return nil, err
		}
		return p, nil
	},
	PKValue: func(p *AuthorProfile) any {
		return p.ID
	},
	ColumnValue: func(p *AuthorProfile, idx int) any {
		switch idx {
		case 0:
			return p.ID
		case 1:
			return p.Bio
		case 2:
			return p.AuthorID
		case 3:
			return p.CreatedAt
		case 4:
			return p.UpdatedAt
		}
		return nil
	},
	Snapshot: func(p *AuthorProfile) any {
		return p.Bio
	},
	Diff: func(p *AuthorProfile, snap any) []drel.FieldChange {
		if p.Bio != snap.(string) {
			return []drel.FieldChange{{Column: "bio", Value: p.Bio}}
		}
		return nil
	},
	InsertColumns: func(p *AuthorProfile) ([]string, []any) {
		return []string{"bio", "author_id"}, []any{p.Bio, p.AuthorID}
	},
	ScanReturning: func(p *AuthorProfile, row drel.Row) error {
		return row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	},
}

// --- Meta base conversions for RelationInfo ---

var bookMetaBase = drel.ToMetaBase(&bookMeta)
var profileMetaBase = drel.ToMetaBase(&profileMeta)
var authorMetaBase = drel.ToMetaBase(&authorMeta)

// --- RelationInfo definitions ---

var booksRelation = &drel.RelationInfo{
	Name:        "Books",
	Type:        drel.HasMany,
	FKColumn:    "author_id",
	RelatedMeta: bookMetaBase,
	FieldSetter: func(parent any, related any) {
		a := parent.(*Author)
		items := related.([]any)
		books := make([]*Book, len(items))
		for i, item := range items {
			books[i] = item.(*Book)
		}
		a.Books = books
	},
}

var profileRelation = &drel.RelationInfo{
	Name:        "Profile",
	Type:        drel.HasOne,
	FKColumn:    "author_id",
	RelatedMeta: profileMetaBase,
	FieldSetter: func(parent any, related any) {
		a := parent.(*Author)
		a.Profile = related.(*AuthorProfile)
	},
}

var authorRelation = &drel.RelationInfo{
	Name:        "Author",
	Type:        drel.BelongsTo,
	FKColumn:    "author_id",
	RelatedMeta: authorMetaBase,
	FieldSetter: func(parent any, related any) {
		b := parent.(*Book)
		b.Author = related.(*Author)
	},
}

// --- Test helpers ---

func setupRelationDB(t *testing.T) *drel.Engine {
	t.Helper()
	engine := setupTestDB(t)
	ctx := context.Background()
	drv := engine.Driver()

	_, err := drv.Exec(ctx, `
		CREATE TABLE authors (
			id         SERIAL PRIMARY KEY,
			name       TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	require.NoError(t, err)

	_, err = drv.Exec(ctx, `
		CREATE TABLE books (
			id         SERIAL PRIMARY KEY,
			title      TEXT NOT NULL,
			author_id  INTEGER NOT NULL REFERENCES authors(id),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	require.NoError(t, err)

	_, err = drv.Exec(ctx, `
		CREATE TABLE author_profiles (
			id         SERIAL PRIMARY KEY,
			bio        TEXT NOT NULL,
			author_id  INTEGER NOT NULL UNIQUE REFERENCES authors(id),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	require.NoError(t, err)

	return engine
}

func seedRelationData(t *testing.T, engine *drel.Engine) {
	t.Helper()
	ctx := context.Background()
	drv := engine.Driver()

	// Authors
	_, err := drv.Exec(ctx, "INSERT INTO authors (id, name) VALUES (1, 'Alice'), (2, 'Bob')")
	require.NoError(t, err)

	// Books
	_, err = drv.Exec(ctx, `
		INSERT INTO books (title, author_id) VALUES
			('Book A1', 1),
			('Book A2', 1),
			('Book A3', 1),
			('Book B1', 2)
	`)
	require.NoError(t, err)

	// Profiles
	_, err = drv.Exec(ctx, `
		INSERT INTO author_profiles (bio, author_id) VALUES
			('Alice writes things', 1),
			('Bob also writes', 2)
	`)
	require.NoError(t, err)
}

// --- Tests ---

func TestIntegration_Include_HasMany(t *testing.T) {
	engine := setupRelationDB(t)
	seedRelationData(t, engine)
	repo := drel.NewRepository(engine, authorMeta)
	ctx := context.Background()

	alice, err := repo.Include(drel.NewIncludeSpec(booksRelation)).Find(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "Alice", alice.Name)
	require.Len(t, alice.Books, 3)

	titles := make([]string, len(alice.Books))
	for i, b := range alice.Books {
		titles[i] = b.Title
	}
	assert.ElementsMatch(t, []string{"Book A1", "Book A2", "Book A3"}, titles)
}

func TestIntegration_Include_HasMany_Empty(t *testing.T) {
	engine := setupRelationDB(t)
	ctx := context.Background()
	drv := engine.Driver()

	// Insert an author with no books.
	_, err := drv.Exec(ctx, "INSERT INTO authors (id, name) VALUES (1, 'Lonely')")
	require.NoError(t, err)

	repo := drel.NewRepository(engine, authorMeta)
	author, err := repo.Include(drel.NewIncludeSpec(booksRelation)).Find(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "Lonely", author.Name)
	assert.NotNil(t, author.Books)
	assert.Len(t, author.Books, 0)
}

func TestIntegration_Include_HasOne(t *testing.T) {
	engine := setupRelationDB(t)
	seedRelationData(t, engine)
	repo := drel.NewRepository(engine, authorMeta)
	ctx := context.Background()

	alice, err := repo.Include(drel.NewIncludeSpec(profileRelation)).Find(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "Alice", alice.Name)
	require.NotNil(t, alice.Profile)
	assert.Equal(t, "Alice writes things", alice.Profile.Bio)
	assert.Equal(t, 1, alice.Profile.AuthorID)
}

func TestIntegration_Include_BelongsTo(t *testing.T) {
	engine := setupRelationDB(t)
	seedRelationData(t, engine)
	bookRepo := drel.NewRepository(engine, bookMeta)
	ctx := context.Background()

	book, err := bookRepo.Include(drel.NewIncludeSpec(authorRelation)).Find(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "Book A1", book.Title)
	require.NotNil(t, book.Author)
	assert.Equal(t, "Alice", book.Author.Name)
	assert.Equal(t, 1, book.Author.ID)
}

func TestIntegration_Include_MultipleParents(t *testing.T) {
	engine := setupRelationDB(t)
	seedRelationData(t, engine)
	repo := drel.NewRepository(engine, authorMeta)
	ctx := context.Background()

	authors, err := repo.Include(drel.NewIncludeSpec(booksRelation)).All(ctx)
	require.NoError(t, err)
	require.Len(t, authors, 2)

	// Build a map for easier assertions (order may vary).
	booksByAuthor := make(map[string]int)
	for _, a := range authors {
		booksByAuthor[a.Name] = len(a.Books)
	}
	assert.Equal(t, 3, booksByAuthor["Alice"])
	assert.Equal(t, 1, booksByAuthor["Bob"])
}
