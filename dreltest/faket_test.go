package dreltest_test

import "testing"

// fakeT wraps *testing.T but intercepts Fatalf/FailNow so a helper's failure
// path can be asserted without aborting the real test. FailNow panics to unwind
// the helper goroutine, matching testing.T semantics closely enough for asserts.
type fakeT struct {
	*testing.T
	failed bool
	msg    string
}

func (f *fakeT) Fatalf(format string, args ...any) {
	f.failed = true
	f.FailNow()
}

func (f *fakeT) Fatal(args ...any) {
	f.failed = true
	f.FailNow()
}

func (f *fakeT) FailNow() {
	panic("fakeT.FailNow")
}
