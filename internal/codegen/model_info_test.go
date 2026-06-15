package codegen

import "testing"

func TestFieldInfo_MultiColFields(t *testing.T) {
	f := FieldInfo{
		Name:          "balance",
		IsMultiColVO:  true,
		MultiColNames: []string{"balance_amount", "balance_currency"},
		MultiColTypes: []string{"integer", "text"},
	}
	if len(f.MultiColNames) != 2 || f.MultiColNames[0] != "balance_amount" {
		t.Fatalf("MultiColNames not set: %#v", f.MultiColNames)
	}
	if len(f.MultiColTypes) != 2 || f.MultiColTypes[1] != "text" {
		t.Fatalf("MultiColTypes not set: %#v", f.MultiColTypes)
	}
}
