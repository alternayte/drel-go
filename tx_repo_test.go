package drel

import (
	"context"
	"testing"
)

func TestTxRepo_ReturnsTxRepository(t *testing.T) {
	ctx := context.Background()
	eng, err := NewEngine(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	meta := akOrderMeta() // from mutation_appkey_test.go (package drel)
	_, _ = eng.Exec(ctx, `CREATE TABLE ak_orders (id TEXT PRIMARY KEY, name TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`)

	err = eng.Transaction(ctx, func(tx *Tx) error {
		repo := Repo(tx, meta) // sugar for NewTxRepository(tx, meta)
		o := &akOrder{Name: "via-repo"}
		repo.Add(o)
		return tx.SaveChanges(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
}
