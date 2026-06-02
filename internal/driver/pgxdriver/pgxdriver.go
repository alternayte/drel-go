package pgxdriver

import (
	"context"
	"fmt"

	"github.com/alternayte/drel/internal/driver"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxDriver implements driver.Driver using a pgxpool connection pool.
type PgxDriver struct {
	pool *pgxpool.Pool
}

// New creates a new PgxDriver by connecting to the given DSN.
func New(ctx context.Context, dsn string) (*PgxDriver, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxdriver: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgxdriver: ping: %w", err)
	}
	return &PgxDriver{pool: pool}, nil
}

// QueryRow executes a query that returns at most one row.
func (d *PgxDriver) QueryRow(ctx context.Context, sql string, args ...any) driver.Row {
	return d.pool.QueryRow(ctx, sql, args...)
}

// Query executes a query that returns multiple rows.
func (d *PgxDriver) Query(ctx context.Context, sql string, args ...any) (driver.Rows, error) {
	rows, err := d.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &pgxRows{rows: rows}, nil
}

// Exec executes a statement and returns the number of affected rows.
func (d *PgxDriver) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := d.pool.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Begin starts a new database transaction with default options.
func (d *PgxDriver) Begin(ctx context.Context) (driver.Tx, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &pgxTx{tx: tx}, nil
}

// BeginTx starts a new database transaction with the given options.
func (d *PgxDriver) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	pgxOpts := pgx.TxOptions{
		AccessMode: pgx.ReadWrite,
	}
	if opts.ReadOnly {
		pgxOpts.AccessMode = pgx.ReadOnly
	}
	switch opts.Isolation {
	case driver.IsoReadCommitted:
		pgxOpts.IsoLevel = pgx.ReadCommitted
	case driver.IsoRepeatableRead:
		pgxOpts.IsoLevel = pgx.RepeatableRead
	case driver.IsoSerializable:
		pgxOpts.IsoLevel = pgx.Serializable
	}
	tx, err := d.pool.BeginTx(ctx, pgxOpts)
	if err != nil {
		return nil, err
	}
	return &pgxTx{tx: tx}, nil
}

// SendBatch sends all queued queries in a single pipelined round-trip using the
// pgx batch protocol. Results must be read in order via the returned
// BatchResults before it is closed.
func (d *PgxDriver) SendBatch(ctx context.Context, items []driver.BatchItem) (driver.BatchResults, error) {
	batch := &pgx.Batch{}
	for _, it := range items {
		batch.Queue(it.SQL, it.Args...)
	}
	br := d.pool.SendBatch(ctx, batch)
	return &pgxBatchResults{br: br}, nil
}

type pgxBatchResults struct {
	br pgx.BatchResults
}

func (b *pgxBatchResults) Query() (driver.Rows, error) {
	rows, err := b.br.Query()
	if err != nil {
		return nil, err
	}
	return &pgxRows{rows: rows}, nil
}

func (b *pgxBatchResults) Close() error { return b.br.Close() }

// Close shuts down the connection pool.
func (d *PgxDriver) Close() {
	d.pool.Close()
}

type pgxRows struct {
	rows interface {
		Next() bool
		Scan(dest ...any) error
		Close()
		Err() error
	}
}

func (r *pgxRows) Next() bool            { return r.rows.Next() }
func (r *pgxRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }
func (r *pgxRows) Close()                { r.rows.Close() }
func (r *pgxRows) Err() error            { return r.rows.Err() }
