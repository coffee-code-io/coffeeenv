package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLnCreatesSymlinkByDefault(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "nested", "dst")
	if err := os.WriteFile(src, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := lnHandler{}
	d, err := h.Decode(RawState{Type: "ln", Name: "ln", Params: map[string]any{"src": src, "dst": dst}})
	if err != nil {
		t.Fatal(err)
	}
	obs, err := h.Read(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	acts, err := h.Diff(d, obs)
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 1 {
		t.Fatalf("want one action, got %d", len(acts))
	}
	if err := h.Apply(context.Background(), acts[0]); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(dst); err != nil || target != src {
		t.Fatalf("readlink = %q, %v; want %q", target, err, src)
	}
	obs, err = h.Read(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	acts, err = h.Diff(d, obs)
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 0 {
		t.Fatalf("want converged, got %d action(s)", len(acts))
	}
}
