package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveAuthToken_Flag(t *testing.T) {
	args := []string{"drel", "migrate", "up", "--auth-token", "tok123"}
	assert.Equal(t, "tok123", resolveAuthToken(args))
}

func TestResolveAuthToken_Env(t *testing.T) {
	t.Setenv("TURSO_AUTH_TOKEN", "envtok")
	assert.Equal(t, "envtok", resolveAuthToken([]string{"drel", "migrate", "up"}))
}

func TestResolveAuthToken_FlagBeatsEnv(t *testing.T) {
	t.Setenv("TURSO_AUTH_TOKEN", "envtok")
	args := []string{"drel", "migrate", "up", "--auth-token", "flagtok"}
	assert.Equal(t, "flagtok", resolveAuthToken(args))
}

func TestResolveAuthToken_None(t *testing.T) {
	_ = os.Unsetenv("TURSO_AUTH_TOKEN")
	assert.Equal(t, "", resolveAuthToken([]string{"drel", "migrate", "status"}))
}
