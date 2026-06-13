package state

import (
	"context"
	"path/filepath"
	"testing"
)

// TestEnvVisibleToShell: an env state applied before a shell command makes the
// variable visible to that command (it runs as a child of this process).
func TestEnvVisibleToShell(t *testing.T) {
	ctx := context.Background()
	out := filepath.Join(t.TempDir(), "out")

	env := &envDesired{Name: "COFFEEENV_TEST_FOO", Value: "bar", Target: filepath.Join(t.TempDir(), "env.sh")}
	if err := (envHandler{}).Apply(ctx, Action{Payload: *env}); err != nil {
		t.Fatalf("env apply: %v", err)
	}
	if err := (shellHandler{}).Apply(ctx, Action{Payload: "printf %s \"$COFFEEENV_TEST_FOO\" > " + out}); err != nil {
		t.Fatalf("shell apply: %v", err)
	}
	if got := readFile(t, out); got != "bar" {
		t.Errorf("shell saw COFFEEENV_TEST_FOO=%q, want bar", got)
	}

	// expand: PATH-style prepend resolves $PATH against the live env.
	envP := &envDesired{Name: "COFFEEENV_TEST_PATHY", Value: "/x:$PATH", Expand: true, Target: filepath.Join(t.TempDir(), "env.sh")}
	if err := (envHandler{}).Apply(ctx, Action{Payload: *envP}); err != nil {
		t.Fatalf("env apply expand: %v", err)
	}
	out2 := filepath.Join(t.TempDir(), "out2")
	if err := (shellHandler{}).Apply(ctx, Action{Payload: "printf %s \"$COFFEEENV_TEST_PATHY\" > " + out2}); err != nil {
		t.Fatalf("shell apply: %v", err)
	}
	if got := readFile(t, out2); got == "/x:$PATH" || got[:3] != "/x:" {
		t.Errorf("expand env not resolved for shell: %q", got)
	}
}
