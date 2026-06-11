package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCopyHandler: a copy state recursively materializes src into dst, is
// idempotent (no actions on a second pass), and re-copies when a file differs.
func TestCopyHandler(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(src, "a.md"), "A")
	mustWrite(t, filepath.Join(src, "sub", "b.md"), "B")

	h := copyHandler{}
	rs := RawState{Type: "copy", Name: "k", Params: map[string]any{"src": src, "dst": dst}}
	d, err := h.Decode(rs)
	if err != nil {
		t.Fatal(err)
	}

	apply := func() int {
		obs, err := h.Read(context.Background(), d)
		if err != nil {
			t.Fatal(err)
		}
		acts, err := h.Diff(d, obs)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range acts {
			if err := h.Apply(context.Background(), a); err != nil {
				t.Fatal(err)
			}
		}
		return len(acts)
	}

	if n := apply(); n != 2 {
		t.Fatalf("first pass: want 2 copy actions, got %d", n)
	}
	if got := readFile(t, filepath.Join(dst, "a.md")); got != "A" {
		t.Errorf("a.md = %q", got)
	}
	if got := readFile(t, filepath.Join(dst, "sub", "b.md")); got != "B" {
		t.Errorf("sub/b.md = %q", got)
	}
	if n := apply(); n != 0 {
		t.Errorf("second pass should be idempotent, got %d actions", n)
	}

	// Drift: change a dst file -> exactly one re-copy.
	mustWrite(t, filepath.Join(dst, "a.md"), "changed")
	if n := apply(); n != 1 {
		t.Errorf("drift: want 1 re-copy, got %d", n)
	}
	if got := readFile(t, filepath.Join(dst, "a.md")); got != "A" {
		t.Errorf("a.md after re-copy = %q", got)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
