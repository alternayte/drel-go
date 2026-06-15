package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTxInclude_LoadsChildrenAndSeesUncommittedWrite(t *testing.T) {
	engine := setupUoWIncludeEngine(t)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		// Insert a second member inside the tx (uncommitted).
		if _, err := tx.Exec(ctx, "INSERT INTO uow_members (team_id, nick) VALUES (1, 'tx-only')"); err != nil {
			return err
		}
		repo := drel.NewTxRepository(tx, uowTeamMeta)
		team, err := repo.Include(membersRel()).Find(ctx, 1)
		if err != nil {
			return err
		}
		// Must see both the committed member and the in-tx member — proving the
		// include sub-query ran on the transaction connection, not the pool.
		require.Len(t, team.Members, 2)
		nicks := []string{team.Members[0].Nick, team.Members[1].Nick}
		assert.Contains(t, nicks, "old-nick")
		assert.Contains(t, nicks, "tx-only")
		return nil
	})
	require.NoError(t, err)
}

func TestTxInclude_EditedChildIsFlushedOnCommit(t *testing.T) {
	engine := setupUoWIncludeEngine(t)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, uowTeamMeta)
		team, err := repo.Include(membersRel()).Find(ctx, 1)
		if err != nil {
			return err
		}
		require.Len(t, team.Members, 1)
		team.Members[0].Nick = "tx-edited"
		// Transaction() flushes the tracker on commit.
		return nil
	})
	require.NoError(t, err)

	memberRepo := drel.NewRepository(engine, uowMemberMeta)
	reread, err := memberRepo.Find(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "tx-edited", reread.Nick, "edited Include'd child inside a Tx must be flushed on commit")
}
