package drel

import (
	"context"
	"testing"
	"time"
)

func TestWithTimeout_Precedence(t *testing.T) {
	t.Run("no engine default returns ctx unchanged", func(t *testing.T) {
		e := &Engine{}
		ctx := context.Background()
		got, cancel := e.withTimeout(ctx)
		defer cancel()
		if _, ok := got.Deadline(); ok {
			t.Fatal("no engine default must not add a deadline")
		}
	})

	t.Run("engine default applied when no caller deadline", func(t *testing.T) {
		e := &Engine{queryTimeout: 50 * time.Millisecond}
		got, cancel := e.withTimeout(context.Background())
		defer cancel()
		dl, ok := got.Deadline()
		if !ok {
			t.Fatal("engine default must add a deadline")
		}
		if d := time.Until(dl); d <= 0 || d > 60*time.Millisecond {
			t.Fatalf("deadline %v not within engine default window", d)
		}
	})

	t.Run("shorter caller deadline wins", func(t *testing.T) {
		e := &Engine{queryTimeout: time.Hour}
		callerCtx, callerCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer callerCancel()
		got, cancel := e.withTimeout(callerCtx)
		defer cancel()
		dl, ok := got.Deadline()
		if !ok {
			t.Fatal("caller deadline must be preserved")
		}
		if d := time.Until(dl); d > 30*time.Millisecond {
			t.Fatalf("shorter caller deadline must win, got %v", d)
		}
	})
}

func TestEngineQueryTimeout_FiresOnSlowExec(t *testing.T) {
	e, err := NewEngine(":memory:", WithQueryTimeout(1*time.Nanosecond))
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer e.Close()
	// 1ns default deadline elapses before the exec runs.
	_, err = e.Exec(context.Background(), "CREATE TABLE t (id INTEGER)")
	if err == nil {
		t.Fatal("expected a deadline-exceeded error from the engine default timeout")
	}
}

// TestEngineQueryTimeout_MultiRowDrainCompletes proves that a generous engine
// default timeout does not cancel the context before the caller finishes
// draining a multi-row result set. This is the regression test for the bug
// where queryRoutedTimeout called defer cancel() and therefore cancelled the
// context the instant it returned the Rows handle.
func TestEngineQueryTimeout_MultiRowDrainCompletes(t *testing.T) {
	// Use a generous timeout — we want to prove rows drain successfully, not
	// that the timeout fires.
	e, err := NewEngine(":memory:", WithQueryTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer e.Close()

	ctx := context.Background()

	// Seed a table with 500 rows.
	if _, err := e.Exec(ctx, "CREATE TABLE nums (n INTEGER)"); err != nil {
		t.Fatalf("CREATE: %v", err)
	}
	for i := 0; i < 500; i++ {
		if _, err := e.Exec(ctx, "INSERT INTO nums (n) VALUES (?)", i); err != nil {
			t.Fatalf("INSERT %d: %v", i, err)
		}
	}

	// Engine.Query — the rows must drain without context.canceled.
	rows, err := e.Query(ctx, "SELECT n FROM nums ORDER BY n")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var n int
		if scanErr := rows.Scan(&n); scanErr != nil {
			t.Fatalf("Scan row %d: %v", count, scanErr)
		}
		count++
	}
	if iterErr := rows.Err(); iterErr != nil {
		t.Fatalf("rows.Err() after drain: %v (engine default timeout must not cancel before Close)", iterErr)
	}
	if count != 500 {
		t.Fatalf("expected 500 rows, got %d", count)
	}
}
