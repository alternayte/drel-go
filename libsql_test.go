package drel

import "testing"

func TestDetectDialect_LibSQL(t *testing.T) {
	cases := map[string]string{
		"libsql://db.turso.io":   "libsql",
		"wss://db.turso.io":      "libsql",
		"ws://localhost:8080":    "libsql",
		"file:app.db":            "sqlite",
		":memory:":               "sqlite",
		"postgres://localhost/x": "postgres",
	}
	for dsn, want := range cases {
		if got := detectDialect(dsn); got != want {
			t.Errorf("detectDialect(%q) = %q, want %q", dsn, got, want)
		}
	}
}

func TestApplyAuthToken(t *testing.T) {
	if got := applyAuthToken("libsql://db.turso.io", "tok"); got != "libsql://db.turso.io?authToken=tok" {
		t.Errorf("got %q", got)
	}
	if got := applyAuthToken("libsql://db.turso.io?x=1", "tok"); got != "libsql://db.turso.io?x=1&authToken=tok" {
		t.Errorf("got %q", got)
	}
	// No token → unchanged.
	if got := applyAuthToken("libsql://db.turso.io", ""); got != "libsql://db.turso.io" {
		t.Errorf("got %q", got)
	}
	// Existing token → unchanged.
	if got := applyAuthToken("libsql://db.turso.io?authToken=abc", "tok"); got != "libsql://db.turso.io?authToken=abc" {
		t.Errorf("got %q", got)
	}
}
