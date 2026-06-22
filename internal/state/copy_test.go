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

// TestCopyHandlerSkipsMetadata: a copy never installs coffeeenv-internal
// scaffolding (cue.mod/, manifest.json, coffeeenv.lock.json) — so a pulled skill
// dir copies only its real content.
func TestCopyHandlerDstFile(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", "tool")
	dst := filepath.Join(root, "bin", "renamed")
	mustWrite(t, src, "tool")

	h := copyHandler{}
	d, err := h.Decode(RawState{Type: "copy", Name: "k", Params: map[string]any{"src": src, "dst": dst, "dst_file": true}})
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
	if got := readFile(t, dst); got != "tool" {
		t.Fatalf("dst file = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dst, filepath.Base(src))); err == nil {
		t.Fatalf("dst_file should not create nested basename")
	}
}

func TestCopyHandlerHostRequiresAbsoluteSrc(t *testing.T) {
	_, err := copyHandler{}.Decode(RawState{Type: "copy", Name: "k", Params: map[string]any{"src": "relative", "dst": "/tmp/x", "host": true}})
	if err == nil {
		t.Fatal("expected host copy with relative src to fail")
	}
}

func TestCopyHandlerPermOverride(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	mustWrite(t, filepath.Join(src, "tool"), "tool")

	h := copyHandler{}
	d, err := h.Decode(RawState{Type: "copy", Name: "k", Params: map[string]any{"src": src, "dst": dst, "perm": float64(0o755)}})
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
	if info, err := os.Stat(filepath.Join(dst, "tool")); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("file perm = %v, %v; want 0755", info, err)
	}
}

func TestCopyHandlerSkipsMetadata(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	mustWrite(t, filepath.Join(src, "SKILL.md"), "skill")
	mustWrite(t, filepath.Join(src, "manifest.json"), `{"type":"skill"}`)
	mustWrite(t, filepath.Join(src, "coffeeenv.lock.json"), `{}`)
	mustWrite(t, filepath.Join(src, "cue.mod", "module.cue"), `module: "x"`)

	h := copyHandler{}
	d, err := h.Decode(RawState{Type: "copy", Name: "k", Params: map[string]any{"src": src, "dst": dst}})
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
		t.Fatalf("want 1 copy action (only SKILL.md), got %d", len(acts))
	}
	for _, a := range acts {
		if err := h.Apply(context.Background(), a); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md should be copied: %v", err)
	}
	for _, skip := range []string{"manifest.json", "coffeeenv.lock.json", filepath.Join("cue.mod", "module.cue")} {
		if _, err := os.Stat(filepath.Join(dst, skip)); err == nil {
			t.Errorf("%s should not be copied", skip)
		}
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
