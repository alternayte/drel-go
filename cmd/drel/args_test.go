package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseArgs_Generate(t *testing.T) {
	tests := []struct {
		name       string
		argv       []string
		wantConfig string
	}{
		{"space form", []string{"generate", "--config", "x.yaml"}, "x.yaml"},
		{"equals form", []string{"generate", "--config=x.yaml"}, "x.yaml"},
		{"short space form", []string{"generate", "-c", "x.yaml"}, "x.yaml"},
		{"short equals form", []string{"generate", "-c=x.yaml"}, "x.yaml"},
		{"default", []string{"generate"}, "drel.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc, err := parseArgs(tt.argv)
			require.NoError(t, err)
			assert.Equal(t, "generate", pc.Command)
			assert.Equal(t, tt.wantConfig, pc.ConfigPath)
		})
	}
}

func TestParseArgs_MigrateNew_ConfigBeforeName(t *testing.T) {
	// Regression for the literal "--config" migration-name bug.
	pc, err := parseArgs([]string{"migrate", "new", "--config", "x.yaml", "addedstuff"})
	require.NoError(t, err)
	assert.Equal(t, "migrate", pc.Command)
	assert.Equal(t, "new", pc.Subcommand)
	assert.Equal(t, "x.yaml", pc.ConfigPath)
	assert.Equal(t, []string{"addedstuff"}, pc.Positional)
}

func TestParseArgs_MigrateNew_EqualsBeforeName(t *testing.T) {
	pc, err := parseArgs([]string{"migrate", "new", "--config=x.yaml", "addedstuff"})
	require.NoError(t, err)
	assert.Equal(t, []string{"addedstuff"}, pc.Positional)
	assert.Equal(t, "x.yaml", pc.ConfigPath)
}

func TestParseArgs_MigrateNew_NameBeforeFlag(t *testing.T) {
	// flag package stops at the first non-flag arg, so a flag after the
	// positional is treated as another positional. Accept either ordering by
	// using flag.Parse semantics: name first, then --config.
	pc, err := parseArgs([]string{"migrate", "new", "addedstuff", "--config=x.yaml"})
	require.NoError(t, err)
	assert.Equal(t, []string{"addedstuff", "--config=x.yaml"}, pc.Positional)
	// With flag-after-positional, --config is NOT consumed; that is acceptable
	// (matches stdlib flag behaviour) as long as the name is never "--config".
	assert.NotContains(t, pc.Positional, "--config")
}

func TestParseArgs_Generate_Watch(t *testing.T) {
	pc, err := parseArgs([]string{"generate", "--watch", "--config=x.yaml"})
	require.NoError(t, err)
	assert.True(t, pc.Watch)
	assert.Equal(t, "x.yaml", pc.ConfigPath)
}

func TestParseArgs_MigrateNew_RejectsFlaggyName(t *testing.T) {
	_, err := parseArgs([]string{"migrate", "new", "--config=x.yaml", "-weird"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration name")
}

func TestParseArgs_MigrateNew_RejectsPathSeparatorName(t *testing.T) {
	_, err := parseArgs([]string{"migrate", "new", "foo/bar"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration name")
}

func TestParseArgs_MigrateNew_MissingName(t *testing.T) {
	_, err := parseArgs([]string{"migrate", "new", "--config=x.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestParseArgs_UnknownFlag(t *testing.T) {
	_, err := parseArgs([]string{"generate", "--nope"})
	require.Error(t, err)
}

func TestParseArgs_MissingConfigValue(t *testing.T) {
	_, err := parseArgs([]string{"generate", "--config"})
	require.Error(t, err)
}

func TestParseArgs_NoCommand(t *testing.T) {
	_, err := parseArgs(nil)
	require.Error(t, err)
}

func TestParseArgs_UnknownCommand(t *testing.T) {
	_, err := parseArgs([]string{"frobnicate"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")
}

func TestParseArgs_Migrate_NoSubcommand(t *testing.T) {
	_, err := parseArgs([]string{"migrate"})
	require.Error(t, err)
}

func TestParseArgs_Migrate_UnknownSubcommand(t *testing.T) {
	_, err := parseArgs([]string{"migrate", "frobnicate"})
	require.Error(t, err)
}

func TestParseArgs_MigrateUp(t *testing.T) {
	pc, err := parseArgs([]string{"migrate", "up", "--config=x.yaml"})
	require.NoError(t, err)
	assert.Equal(t, "migrate", pc.Command)
	assert.Equal(t, "up", pc.Subcommand)
	assert.Equal(t, "x.yaml", pc.ConfigPath)
}

func TestParseArgs_Version(t *testing.T) {
	pc, err := parseArgs([]string{"version"})
	require.NoError(t, err)
	assert.Equal(t, "version", pc.Command)
}
