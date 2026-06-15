package codegen

import "testing"

// TestBuildTable_NullableVOHasIsZero verifies that a single-column VO whose type
// defines IsZero() bool produces a nullable column in the generated DDL.
// HasIsZero means Value() returns nil for the zero value; the column must
// accept NULL or every zero-VO insert will violate NOT NULL.
func TestBuildTable_NullableVOHasIsZero(t *testing.T) {
	m := ModelInfo{
		Name: "Account", TableName: "accounts", PKType: "int",
		Fields: []FieldInfo{
			// email has IsZero -> nullable (zero email -> Value()=nil -> SQL NULL)
			{Name: "email", GoType: "models.Email", ColumnName: "email", LocalGoType: "Email",
				IsVO: true, VOBaseType: "string", HasIsZero: true, IsComparable: true},
			// balance has no IsZero -> NOT NULL
			{Name: "balance", GoType: "models.Cents", ColumnName: "balance", LocalGoType: "Cents",
				IsVO: true, VOBaseType: "int64", IsComparable: true},
		},
	}

	pg := buildTable(m, nil, "postgres")

	var emailCol, balanceCol *Column
	for i := range pg.Columns {
		switch pg.Columns[i].Name {
		case "email":
			emailCol = &pg.Columns[i]
		case "balance":
			balanceCol = &pg.Columns[i]
		}
	}
	if emailCol == nil {
		t.Fatal("email column missing from table")
	}
	if balanceCol == nil {
		t.Fatal("balance column missing from table")
	}
	if emailCol.NotNull {
		t.Errorf("email (HasIsZero=true) should be nullable (NotNull=false), got NotNull=true")
	}
	if !balanceCol.NotNull {
		t.Errorf("balance (HasIsZero=false) should be NOT NULL (NotNull=true), got NotNull=false")
	}
}

func TestBuildTable_MultiColVOColumns(t *testing.T) {
	m := ModelInfo{
		Name:      "Account",
		TableName: "accounts",
		PKType:    "int",
		Fields: []FieldInfo{
			{Name: "name", GoType: "string", ColumnName: "name", LocalGoType: "string"},
			{
				Name:          "balance",
				GoType:        "testmod/models.Money",
				ColumnName:    "balance_amount",
				LocalGoType:   "Money",
				IsMultiColVO:  true,
				MultiColNames: []string{"balance_amount", "balance_currency"},
				MultiColTypes: []string{"text", "text"},
			},
		},
	}

	tbl := buildTable(m, nil, "postgres")

	var got []string
	for _, c := range tbl.Columns {
		got = append(got, c.Name+":"+c.Type)
	}
	// id, name, balance_amount, balance_currency, created_at, updated_at
	want := []string{
		"id:SERIAL PRIMARY KEY",
		"name:text",
		"balance_amount:text",
		"balance_currency:text",
		"created_at:timestamptz",
		"updated_at:timestamptz",
	}
	if len(got) != len(want) {
		t.Fatalf("column count: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("column %d: got %q want %q (all: %v)", i, got[i], want[i], got)
		}
	}
	// Both sub-columns NOT NULL (the VO field is a value type, not a pointer).
	for _, c := range tbl.Columns {
		if c.Name == "balance_amount" || c.Name == "balance_currency" {
			if !c.NotNull {
				t.Fatalf("sub-column %s should be NOT NULL", c.Name)
			}
		}
	}
}
