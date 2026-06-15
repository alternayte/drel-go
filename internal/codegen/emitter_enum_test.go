package codegen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmitModelFile_StringEnumValidators(t *testing.T) {
	m := ModelInfo{
		Name: "User", PkgName: "users", PKType: "int", TableName: "users",
		Fields: []FieldInfo{
			{Name: "role", GoType: "users.Role", ColumnName: "role", LocalGoType: "Role",
				IsEnum: true, EnumBaseType: "string", EnumValues: []string{"admin", "user"}},
		},
	}
	out := EmitModelFile(m)

	assert.Contains(t, out, "func (r Role) IsValid() bool {")
	assert.Contains(t, out, "func RoleValues() []Role {")
	// String enum values are quoted literals.
	assert.Contains(t, out, `Role("admin")`)
	assert.Contains(t, out, `Role("user")`)
}

func TestEmitModelFile_IntEnumValidators(t *testing.T) {
	m := ModelInfo{
		Name: "Ticket", PkgName: "tickets", PKType: "int", TableName: "tickets",
		Fields: []FieldInfo{
			{Name: "priority", GoType: "tickets.Priority", ColumnName: "priority", LocalGoType: "Priority",
				IsEnum: true, EnumIsInt: true, EnumBaseType: "int", EnumValues: []string{"0", "1", "2"}},
		},
	}
	out := EmitModelFile(m)

	assert.Contains(t, out, "func (r Priority) IsValid() bool {")
	assert.Contains(t, out, "func PriorityValues() []Priority {")
	// Int enum values are bare numeric literals.
	assert.Contains(t, out, "Priority(0)")
	assert.Contains(t, out, "Priority(2)")
	assert.NotContains(t, out, `Priority("0")`)
}

func TestEmitModelFile_DedupesRepeatedEnumType(t *testing.T) {
	m := ModelInfo{
		Name: "Edge", PkgName: "graph", PKType: "int", TableName: "edges",
		Fields: []FieldInfo{
			{Name: "from", GoType: "graph.State", ColumnName: "from_state", LocalGoType: "State",
				IsEnum: true, EnumBaseType: "string", EnumValues: []string{"open", "closed"}},
			{Name: "to", GoType: "graph.State", ColumnName: "to_state", LocalGoType: "State",
				IsEnum: true, EnumBaseType: "string", EnumValues: []string{"open", "closed"}},
		},
	}
	out := EmitModelFile(m)
	assert.Equal(t, 1, strings.Count(out, "func (r State) IsValid() bool {"))
	assert.Equal(t, 1, strings.Count(out, "func StateValues() []State {"))
}
