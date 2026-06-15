package drel

import (
	"context"
	"errors"
	"testing"

	"github.com/alternayte/drel/internal/dberr"
	"github.com/alternayte/drel/internal/driver"
)

// commitErrTx is a driver.Tx whose Commit returns a fixed error.
type commitErrTx struct{ commitErr error }

func (t *commitErrTx) QueryRow(ctx context.Context, sql string, args ...any) driver.Row {
	return nil
}
func (t *commitErrTx) Query(ctx context.Context, sql string, args ...any) (driver.Rows, error) {
	return nil, nil
}
func (t *commitErrTx) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	return 0, nil
}
func (t *commitErrTx) Commit(ctx context.Context) error   { return t.commitErr }
func (t *commitErrTx) Rollback(ctx context.Context) error { return nil }

// commitErrDriver hands out commitErrTx from Begin/BeginTx.
type commitErrDriver struct{ commitErr error }

func (d *commitErrDriver) QueryRow(ctx context.Context, sql string, args ...any) driver.Row {
	return nil
}
func (d *commitErrDriver) Query(ctx context.Context, sql string, args ...any) (driver.Rows, error) {
	return nil, nil
}
func (d *commitErrDriver) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	return 0, nil
}
func (d *commitErrDriver) Begin(ctx context.Context) (driver.Tx, error) {
	return &commitErrTx{commitErr: d.commitErr}, nil
}
func (d *commitErrDriver) BeginTx(ctx context.Context, o driver.TxOptions) (driver.Tx, error) {
	return &commitErrTx{commitErr: d.commitErr}, nil
}
func (d *commitErrDriver) Ping(ctx context.Context) error { return nil }
func (d *commitErrDriver) Stat() driver.PoolStat          { return driver.PoolStat{} }
func (d *commitErrDriver) Close()                         {}

func TestTransaction_CommitErrorClassified(t *testing.T) {
	// A libSQL-style busy surfaced at COMMIT must classify as serialization.
	drv := &commitErrDriver{commitErr: errors.New("database is locked")}
	e := &Engine{drv: drv}
	err := e.Transaction(context.Background(), func(tx *Tx) error { return nil })
	if !errors.Is(err, dberr.ErrSerializationFailure) {
		t.Fatalf("commit error must classify as ErrSerializationFailure, got %v", err)
	}
}

func TestSaveChanges_CommitErrorClassified(t *testing.T) {
	drv := &commitErrDriver{commitErr: errors.New("database is locked")}
	e := &Engine{drv: drv}
	uow := e.NewUnitOfWork()
	err := uow.SaveChanges(context.Background())
	if !errors.Is(err, dberr.ErrSerializationFailure) {
		t.Fatalf("SaveChanges commit error must classify as ErrSerializationFailure, got %v", err)
	}
}
