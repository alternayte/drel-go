package drel

import (
	"context"
	"testing"

	dialectsqlite "github.com/alternayte/drel/internal/dialect/sqlite"
	"github.com/alternayte/drel/internal/driver"
)

// The sequential fallback path is exercised by recDriver, which does not
// implement driver.Pipeliner, so Execute takes the sequential branch. recDriver
// returns (nil, nil) from Query, so the handler panics when it calls Next() on a
// nil Rows; the routing call is recorded BEFORE that panic, so each Execute is
// wrapped in a recover and we assert only on the recorded target.
func runBatch(b *Batch) {
	defer func() { _ = recover() }()
	_ = b.Execute(context.Background())
}

func TestBatch_SequentialRoutesToReplica_WhenNoItemForcesPrimary(t *testing.T) {
	var calls []string
	primary := &recDriver{name: "primary", calls: &calls}
	r1 := &recDriver{name: "r1", calls: &calls}
	e := &Engine{drv: primary, replicas: []driver.Driver{r1}, dia: dialectsqlite.New()}
	repo := NewRepository(e, routingModelMeta())

	b := e.NewBatch()
	_ = BatchCount(b, repo.newBuilder())
	runBatch(b)

	if len(calls) == 0 || calls[0] != "r1:query" {
		t.Fatalf("batch read should route to replica by default, got %v", calls)
	}
}

func TestBatch_SequentialRoutesToPrimary_WhenAnyItemForcesPrimary(t *testing.T) {
	var calls []string
	primary := &recDriver{name: "primary", calls: &calls}
	r1 := &recDriver{name: "r1", calls: &calls}
	e := &Engine{drv: primary, replicas: []driver.Driver{r1}, dia: dialectsqlite.New()}
	repo := NewRepository(e, routingModelMeta())

	b := e.NewBatch()
	_ = BatchCount(b, repo.newBuilder())           // replica-eligible
	_ = BatchCount(b, repo.newBuilder().Primary()) // forces primary
	runBatch(b)

	for _, c := range calls {
		if c == "r1:query" {
			t.Fatalf("when any batched query forces Primary, the whole batch must hit primary, got %v", calls)
		}
	}
	if len(calls) == 0 {
		t.Fatalf("expected batch to execute, got no calls")
	}
}
