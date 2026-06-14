package drel

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type scanDTO struct {
	Name  string `db:"name"`
	Age   int    `db:"age"`
	Skip  string // no tag — should be ignored
	Dash  string `db:"-"` // explicit skip
	Email string `db:"email"`
}

func TestBuildScanPlan_PicksTaggedFields(t *testing.T) {
	plan := buildScanPlan(reflect.TypeOf(scanDTO{}))

	if len(plan.fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(plan.fields))
	}

	want := []scanField{
		{column: "name", index: 0},
		{column: "age", index: 1},
		{column: "email", index: 4},
	}
	for i, f := range plan.fields {
		if f.column != want[i].column {
			t.Errorf("field %d: column = %q, want %q", i, f.column, want[i].column)
		}
		if f.index != want[i].index {
			t.Errorf("field %d: index = %d, want %d", i, f.index, want[i].index)
		}
	}
}

func TestBuildScanPlan_PointerType(t *testing.T) {
	plan := buildScanPlan(reflect.TypeOf((*scanDTO)(nil)))

	if len(plan.fields) != 3 {
		t.Fatalf("expected 3 fields for pointer type, got %d", len(plan.fields))
	}
}

func TestScanPlan_Columns(t *testing.T) {
	plan := buildScanPlan(reflect.TypeOf(scanDTO{}))
	cols := plan.columns()
	want := []string{"name", "age", "email"}
	if len(cols) != len(want) {
		t.Fatalf("columns len = %d, want %d", len(cols), len(want))
	}
	for i, c := range cols {
		if c != want[i] {
			t.Errorf("columns[%d] = %q, want %q", i, c, want[i])
		}
	}
}

func TestScanPlan_ScanDest(t *testing.T) {
	plan := buildScanPlan(reflect.TypeOf(scanDTO{}))
	dto := &scanDTO{Name: "alice", Age: 30, Email: "alice@example.com"}
	v := reflect.ValueOf(dto)
	dests := plan.scanDest(v)

	if len(dests) != 3 {
		t.Fatalf("scanDest len = %d, want 3", len(dests))
	}

	// Verify the destinations point to the correct fields.
	namePtr, ok := dests[0].(*string)
	if !ok {
		t.Fatalf("dests[0] is %T, want *string", dests[0])
	}
	if *namePtr != "alice" {
		t.Errorf("dests[0] = %q, want %q", *namePtr, "alice")
	}

	agePtr, ok := dests[1].(*int)
	if !ok {
		t.Fatalf("dests[1] is %T, want *int", dests[1])
	}
	if *agePtr != 30 {
		t.Errorf("dests[1] = %d, want %d", *agePtr, 30)
	}

	emailPtr, ok := dests[2].(*string)
	if !ok {
		t.Fatalf("dests[2] is %T, want *string", dests[2])
	}
	if *emailPtr != "alice@example.com" {
		t.Errorf("dests[2] = %q, want %q", *emailPtr, "alice@example.com")
	}
}

func TestGetScanPlan_Caches(t *testing.T) {
	// Clear the cache first to avoid pollution from other tests.
	scanPlanMu.Lock()
	delete(scanPlanCache, reflect.TypeOf(scanDTO{}))
	scanPlanMu.Unlock()

	plan1 := getScanPlan(reflect.TypeOf(scanDTO{}))
	plan2 := getScanPlan(reflect.TypeOf(scanDTO{}))

	if plan1 != plan2 {
		t.Error("getScanPlan did not return the same cached plan on second call")
	}
}

type emptyDTO struct {
	X string
	Y int
}

func TestBuildScanPlan_NoTaggedFields(t *testing.T) {
	plan := buildScanPlan(reflect.TypeOf(emptyDTO{}))
	if len(plan.fields) != 0 {
		t.Fatalf("expected 0 fields, got %d", len(plan.fields))
	}
	if len(plan.columns()) != 0 {
		t.Fatalf("expected 0 columns, got %d", len(plan.columns()))
	}
}

func TestBuildScanPlan_ByColumnIndex(t *testing.T) {
	plan := buildScanPlan(reflect.TypeOf(scanDTO{}))

	// byColumn must map each db tag to that field's within-fields position.
	want := map[string]int{
		"name":  0,
		"age":   1,
		"email": 2,
	}
	if len(plan.byColumn) != len(want) {
		t.Fatalf("byColumn len = %d, want %d", len(plan.byColumn), len(want))
	}
	for col, idx := range want {
		got, ok := plan.byColumn[col]
		if !ok {
			t.Fatalf("byColumn missing column %q", col)
		}
		if got != idx {
			t.Errorf("byColumn[%q] = %d, want %d", col, got, idx)
		}
	}

	// Untagged and db:"-" fields must not appear.
	if _, ok := plan.byColumn["Skip"]; ok {
		t.Error("byColumn should not contain untagged field Skip")
	}
	if _, ok := plan.byColumn["-"]; ok {
		t.Error("byColumn should not contain db:\"-\" field")
	}
}

func TestBuildScanPlan_NoTaggedFields_ByColumnEmpty(t *testing.T) {
	plan := buildScanPlan(reflect.TypeOf(emptyDTO{}))
	if len(plan.byColumn) != 0 {
		t.Fatalf("expected empty byColumn, got %d entries", len(plan.byColumn))
	}
}

func TestScanDestFor_OrdersByColumnNames(t *testing.T) {
	plan := buildScanPlan(reflect.TypeOf(scanDTO{}))
	dto := &scanDTO{}
	v := reflect.ValueOf(dto)

	// Request columns in an order different from struct declaration order.
	dests, err := plan.scanDestFor(v, []string{"email", "name", "age"})
	if err != nil {
		t.Fatalf("scanDestFor returned error: %v", err)
	}
	if len(dests) != 3 {
		t.Fatalf("dests len = %d, want 3", len(dests))
	}

	// dests[0] must address the Email field, dests[1] Name, dests[2] Age.
	emailPtr, ok := dests[0].(*string)
	if !ok {
		t.Fatalf("dests[0] is %T, want *string (Email)", dests[0])
	}
	*emailPtr = "e@x.com"
	namePtr, ok := dests[1].(*string)
	if !ok {
		t.Fatalf("dests[1] is %T, want *string (Name)", dests[1])
	}
	*namePtr = "alice"
	agePtr, ok := dests[2].(*int)
	if !ok {
		t.Fatalf("dests[2] is %T, want *int (Age)", dests[2])
	}
	*agePtr = 30

	if dto.Email != "e@x.com" {
		t.Errorf("Email = %q, want %q", dto.Email, "e@x.com")
	}
	if dto.Name != "alice" {
		t.Errorf("Name = %q, want %q", dto.Name, "alice")
	}
	if dto.Age != 30 {
		t.Errorf("Age = %d, want %d", dto.Age, 30)
	}
}

func TestScanDestFor_UnknownColumnErrors(t *testing.T) {
	plan := buildScanPlan(reflect.TypeOf(scanDTO{}))
	v := reflect.ValueOf(&scanDTO{})

	_, err := plan.scanDestFor(v, []string{"name", "nonexistent"})
	if err == nil {
		t.Fatal("scanDestFor: expected error for unknown column, got nil")
	}
	if !errors.Is(err, ErrUnknownProjectionColumn) {
		t.Errorf("error = %v, want errors.Is ErrUnknownProjectionColumn", err)
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error %q should name the offending column", err.Error())
	}
}
