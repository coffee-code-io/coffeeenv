package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIgnoreFileAppendsOnlyMissingLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	if err := os.WriteFile(path, []byte("existing\nmissing-newline"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := ignoreFileHandler{}
	d, err := h.Decode(RawState{Type: "ignorefile", Name: "ignore", Params: map[string]any{
		"path":  path,
		"lines": []any{"existing", "new"},
	}})
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
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "existing\nmissing-newline\nnew\n" {
		t.Fatalf("content = %q", got)
	}
}
