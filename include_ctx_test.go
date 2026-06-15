package drel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/require"
)

func TestInclude_BatchLoop_RespectsCancelledContext(t *testing.T) {
	engine := setupNestedDB(t) // ni_authors / ni_books fixture
	bookMeta := niBookMeta()
	authorMeta := niAuthorMeta()

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

	// Load the parents with a live context, then cancel before loading includes.
	repo := drel.NewRepository(engine, authorMeta)
	parents, err := repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()).All(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, parents)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before any include batch runs

	_, err = repo.Include(drel.NewIncludeSpec(&authorBooksRel)).
		Where(drel.NewOrderedCol[int]("id").In(1, 2)).
		All(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled), "include loader must surface context.Canceled")
}
