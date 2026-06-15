package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// resolveAuthToken now accepts the token string from parsedCmd.AuthToken;
// these tests verify the precedence logic (explicit flag beats env, env beats
// empty string).

func TestResolveAuthToken_ExplicitToken(t *testing.T) {
	// Explicit token wins regardless of env.
	t.Setenv("TURSO_AUTH_TOKEN", "envtok")
	assert.Equal(t, "tok123", resolveAuthToken("tok123"))
}

func TestResolveAuthToken_Env(t *testing.T) {
	t.Setenv("TURSO_AUTH_TOKEN", "envtok")
	assert.Equal(t, "envtok", resolveAuthToken(""))
}

func TestResolveAuthToken_FlagBeatsEnv(t *testing.T) {
	t.Setenv("TURSO_AUTH_TOKEN", "envtok")
	assert.Equal(t, "flagtok", resolveAuthToken("flagtok"))
}

func TestResolveAuthToken_None(t *testing.T) {
	_ = os.Unsetenv("TURSO_AUTH_TOKEN")
	assert.Equal(t, "", resolveAuthToken(""))
}
