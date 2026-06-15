package codegen

import "testing"

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
