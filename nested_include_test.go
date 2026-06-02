package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Three-level model tree: Author → Books → Reviews ────────────────────────

type niAuthor struct {
	ID    int
	Name  string
	Books []*niBook
}

type niBook struct {
	ID       int
	AuthorID int
	Title    string
	Reviews  []*niReview
}

type niReview struct {
	ID     int
	BookID int
	Stars  int
}

func niAuthorMeta() drel.ModelMeta[niAuthor] {
	return drel.ModelMeta[niAuthor]{
		Table:    "ni_authors",
		Columns:  []string{"id", "name"},
		PKColumn: "id",
		Scan: func(r drel.Row) (*niAuthor, error) {
			a := &niAuthor{}
			return a, r.Scan(&a.ID, &a.Name)
		},
		PKValue:     func(a *niAuthor) any { return a.ID },
		ColumnValue: func(a *niAuthor, i int) any { return [...]any{a.ID, a.Name}[i] },
	}
}

func niBookMeta() drel.ModelMeta[niBook] {
	return drel.ModelMeta[niBook]{
		Table:    "ni_books",
		Columns:  []string{"id", "author_id", "title"},
		PKColumn: "id",
		Scan: func(r drel.Row) (*niBook, error) {
			b := &niBook{}
			return b, r.Scan(&b.ID, &b.AuthorID, &b.Title)
		},
		PKValue:     func(b *niBook) any { return b.ID },
		ColumnValue: func(b *niBook, i int) any { return [...]any{b.ID, b.AuthorID, b.Title}[i] },
	}
}

func niReviewMeta() drel.ModelMeta[niReview] {
	return drel.ModelMeta[niReview]{
		Table:    "ni_reviews",
		Columns:  []string{"id", "book_id", "stars"},
		PKColumn: "id",
		Scan: func(r drel.Row) (*niReview, error) {
			rv := &niReview{}
			return rv, r.Scan(&rv.ID, &rv.BookID, &rv.Stars)
		},
		PKValue:     func(rv *niReview) any { return rv.ID },
		ColumnValue: func(rv *niReview, i int) any { return [...]any{rv.ID, rv.BookID, rv.Stars}[i] },
	}
}

func setupNestedDB(t *testing.T) *drel.Engine {
	t.Helper()
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()
	for _, ddl := range []string{
		`CREATE TABLE ni_authors (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE ni_books (id INTEGER PRIMARY KEY, author_id INTEGER NOT NULL, title TEXT NOT NULL)`,
		`CREATE TABLE ni_reviews (id INTEGER PRIMARY KEY, book_id INTEGER NOT NULL, stars INTEGER NOT NULL)`,
		`INSERT INTO ni_authors (id, name) VALUES (1,'Ann'),(2,'Bob')`,
		`INSERT INTO ni_books (id, author_id, title) VALUES (10,1,'A1'),(11,1,'A2'),(12,2,'B1')`,
		`INSERT INTO ni_reviews (id, book_id, stars) VALUES (100,10,5),(101,10,4),(102,11,3),(103,12,2)`,
	} {
		_, err := engine.Exec(ctx, ddl)
		require.NoError(t, err)
	}
	return engine
}

func TestNestedInclude_ThreeLevels(t *testing.T) {
	engine := setupNestedDB(t)
	ctx := context.Background()

	authorMeta := niAuthorMeta()
	bookMeta := niBookMeta()
	reviewMeta := niReviewMeta()

	bookReviewsRel := drel.RelationInfo{
		Name:        "Reviews",
		Type:        drel.HasMany,
		FKColumn:    "book_id",
		RelatedMeta: drel.ToMetaBase(&reviewMeta),
		FieldSetter: func(parent any, related any) {
			b := parent.(*niBook)
			for _, it := range related.([]any) {
				b.Reviews = append(b.Reviews, it.(*niReview))
			}
		},
	}
	authorBooksRel := drel.RelationInfo{
		Name:        "Books",
		Type:        drel.HasMany,
		FKColumn:    "author_id",
		RelatedMeta: drel.ToMetaBase(&bookMeta),
		FieldSetter: func(parent any, related any) {
			a := parent.(*niAuthor)
			for _, it := range related.([]any) {
				a.Books = append(a.Books, it.(*niBook))
			}
		},
	}

	repo := drel.NewRepository(engine, authorMeta)
	nested := drel.NewIncludeSpec(&authorBooksRel).Then(drel.NewIncludeSpec(&bookReviewsRel))

	authors, err := repo.Include(nested).OrderBy(drel.NewOrderedCol[int]("id").Asc()).All(ctx)
	require.NoError(t, err)
	require.Len(t, authors, 2)

	// Ann → 2 books; A1 has 2 reviews, A2 has 1 review.
	ann := authors[0]
	require.Equal(t, "Ann", ann.Name)
	require.Len(t, ann.Books, 2)

	reviewsByTitle := map[string]int{}
	for _, b := range ann.Books {
		reviewsByTitle[b.Title] = len(b.Reviews)
	}
	assert.Equal(t, 2, reviewsByTitle["A1"])
	assert.Equal(t, 1, reviewsByTitle["A2"])

	// Bob → 1 book with 1 review.
	bob := authors[1]
	require.Len(t, bob.Books, 1)
	assert.Len(t, bob.Books[0].Reviews, 1)
	assert.Equal(t, 2, bob.Books[0].Reviews[0].Stars)
}
