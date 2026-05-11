package codegen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToSnakeCase(t *testing.T) {
	tests := []struct{ input, want string }{
		{"User", "user"},
		{"UserProfile", "user_profile"},
		{"HTMLParser", "html_parser"},
		{"ID", "id"},
		{"UserID", "user_id"},
		{"CreatedAt", "created_at"},
		{"simple", "simple"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, toSnakeCase(tt.input))
		})
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct{ input, want string }{
		{"User", "Users"},
		{"Post", "Posts"},
		{"Address", "Addresses"},
		{"Category", "Categories"},
		{"Key", "Keys"},
		{"Box", "Boxes"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, pluralize(tt.input))
		})
	}
}

func TestSingularize(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"users", "user"},
		{"posts", "post"},
		{"categories", "category"},
		{"companies", "company"},
		{"statuses", "status"},
		{"churches", "church"},
		{"boxes", "box"},
		{"buzzes", "buzz"},
		{"dishes", "dish"},
		{"s", "s"},
		{"", ""},
		{"person", "person"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, singularize(tt.input))
		})
	}
}

func TestInferTableName(t *testing.T) {
	tests := []struct{ input, want string }{
		{"User", "users"},
		{"Post", "posts"},
		{"UserProfile", "user_profiles"},
		{"Category", "categories"},
		{"Address", "addresses"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, inferTableName(tt.input))
		})
	}
}
