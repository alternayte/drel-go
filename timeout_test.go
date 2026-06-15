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
