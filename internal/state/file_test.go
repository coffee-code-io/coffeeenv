package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decodeFile(t *testing.T, params map[string]any) *fileDesired {
	t.Helper()
	d, err := fileHandler{}.Decode(RawState{Type: "file", Name: "f", Params: params})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return d.(*fileDesired)
}

func TestFileRenderJSON(t *testing.T) {
	d := decodeFile(t, map[string]any{
		"path":   "/tmp/x.json",
		"format": "json",
		"data":   map[string]any{"b": 1, "a": 2},
	})
	got := string(d.rendered)
	// Keys must be sorted for a deterministic, idempotent diff.
	if strings.Index(got, `"a"`) > strings.Index(got, `"b"`) {
		t.Errorf("json keys not sorted: %s", got)
	}
}

func TestFileRenderTOML(t *testing.T) {
	d := decodeFile(t, map[string]any{
		"path":   "/tmp/x.toml",
		"format": "toml",
		"data":   map[string]any{"mcp_servers": map[string]any{"fs": map[string]any{"command": "npx"}}},
	})
	if got := string(d.rendered); !strings.Contains(got, "[mcp_servers.fs]") || !strings.Contains(got, "command = 'npx'") {
		t.Errorf("toml render = %q", got)
	}
}

func TestFileRenderDeterministic(t *testing.T) {
	params := map[string]any{
		"path":   "/tmp/x.toml",
		"format": "toml",
		"data":   map[string]any{"z": 1, "a": 2, "m": map[string]any{"k": "v", "j": "w"}},
	}
	a := decodeFile(t, params)
	b := decodeFile(t, params)
	if string(a.rendered) != string(b.rendered) {
		t.Errorf("toml render not deterministic:\n%s\n---\n%s", a.rendered, b.rendered)
	}
}

func TestEnvExpandRoundTrip(t *testing.T) {
	// An expand var renders double-quoted (so $PATH expands when sourced) and
	// reads back as the same value+mode (idempotent).
	v := envVar{Value: "/x/node_modules/.bin:$PATH", Expand: true}
	rendered := quoteShell(v)
	if rendered != `"/x/node_modules/.bin:$PATH"` {
		t.Fatalf("expand render = %s", rendered)
	}
	got, expand := unquoteShell(rendered)
	if got != v.Value || !expand {
		t.Errorf("round-trip = (%q, %v), want (%q, true)", got, expand, v.Value)
	}
	// A normal var stays single-quoted literal.
	if quoteShell(envVar{Value: "nvim"}) != `'nvim'` {
		t.Errorf("literal render = %s", quoteShell(envVar{Value: "nvim"}))
	}
}

func TestFileModeTakesPrecedenceOverPerm(t *testing.T) {
	d := decodeFile(t, map[string]any{
		"path":    "/tmp/x",
		"content": "hi",
		"mode":    float64(0o600),
		"perm":    float64(0o644),
	})
	if d.Mode != 0o600 {
		t.Fatalf("mode = %#o, want 0600", d.Mode)
	}
}

func TestFilePermAndDirPerm(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "x")
	d := decodeFile(t, map[string]any{
		"path":     path,
		"content":  "hi",
		"perm":     float64(0o600),
		"dir_perm": float64(0o700),
	})
	h := fileHandler{}
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
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file perm = %v, %v; want 0600", info, err)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("dir perm = %v, %v; want 0700", info, err)
	}
}

func TestFileContentAndDataConflict(t *testing.T) {
	_, err := fileHandler{}.Decode(RawState{Type: "file", Name: "f", Params: map[string]any{
		"path": "/tmp/x", "content": "hi", "data": map[string]any{"a": 1},
	}})
	if err == nil {
		t.Fatal("expected error when both content and data are set")
	}
}
