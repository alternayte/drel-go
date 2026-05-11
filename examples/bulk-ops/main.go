// Example: bulk-ops
//
// Demonstrates bulk operations that bypass change tracking for efficient
// batch processing. Uses BulkInsert, BulkUpdate, and BulkDelete to operate
// on many rows without the overhead of snapshot diffing.
//
// Key concepts shown:
//   - BulkInsert inserts 100 log entries in a single batch
//   - BulkUpdate changes all INFO entries to WARN using Set clauses
//   - BulkDelete removes all DEBUG entries
//   - All bulk ops return affected row counts
//
// Usage:
//
//	cd examples/bulk-ops
//	go run ../../cmd/drel generate   # generates logs/logentry_drel.go + db/drel_gen.go
//	export DATABASE_URL="postgres://localhost:5432/drelexample?sslmode=disable"
//	go run .
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/examples/bulk-ops/db"
	"github.com/alternayte/drel/examples/bulk-ops/logs"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/drelexample?sslmode=disable"
	}

	ctx := context.Background()

	database, err := db.Open(dsn, drel.WithContext(ctx))
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer database.Close()

	setup(ctx, database)
	defer teardown(ctx, database)

	// === BulkInsert — insert 100 log entries in one batch ===
	fmt.Println("=== BulkInsert ===")
	levels := []string{"INFO", "DEBUG", "ERROR", "WARN"}
	entries := make([]*logs.LogEntry, 100)
	for i := range entries {
		entries[i] = &logs.LogEntry{
			Level:   levels[i%len(levels)],
			Message: fmt.Sprintf("Log message #%d", i+1),
		}
	}

	inserted, err := database.LogEntries.BulkInsert(ctx, entries)
	if err != nil {
		log.Fatalf("bulk insert: %v", err)
	}
	fmt.Printf("  Inserted %d log entries\n", inserted)

	// === BulkUpdate — change all INFO entries to WARN ===
	fmt.Println("\n=== BulkUpdate ===")
	updated, err := database.LogEntries.
		Where(logs.LogEntries.Level.Eq("INFO")).
		BulkUpdate(ctx, drel.Set(logs.LogEntries.Level, "WARN"))
	if err != nil {
		log.Fatalf("bulk update: %v", err)
	}
	fmt.Printf("  Updated %d entries (INFO -> WARN)\n", updated)

	// === BulkDelete — remove all DEBUG entries ===
	fmt.Println("\n=== BulkDelete ===")
	deleted, err := database.LogEntries.
		Where(logs.LogEntries.Level.Eq("DEBUG")).
		BulkDelete(ctx)
	if err != nil {
		log.Fatalf("bulk delete: %v", err)
	}
	fmt.Printf("  Deleted %d DEBUG entries\n", deleted)

	// === Final count ===
	fmt.Println("\n=== Summary ===")
	remaining, err := database.LogEntries.Count(ctx)
	if err != nil {
		log.Fatalf("count: %v", err)
	}
	fmt.Printf("  Remaining entries: %d\n", remaining)
}

func setup(ctx context.Context, database *db.DB) {
	drv := database.Driver()
	drv.Exec(ctx, `DROP TABLE IF EXISTS log_entries`)
	drv.Exec(ctx, `
		CREATE TABLE log_entries (
			id SERIAL PRIMARY KEY,
			level TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
}

func teardown(ctx context.Context, database *db.DB) {
	database.Driver().Exec(ctx, `DROP TABLE IF EXISTS log_entries`)
}
