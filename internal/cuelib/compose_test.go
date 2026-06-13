package cuelib

import (
	"os"
	"path/filepath"
	"testing"
)

// writeChart creates a minimal executable chart dir (cue.mod + env.cue) at a
// module path and returns the dir.
func writeChart(t *testing.T, module, env string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "cue.mod", "module.cue"),
		"module: \""+module+"\"\nlanguage: version: \"v0.9.0\"\n")
	writeFile(t, filepath.Join(dir, "env.cue"), env)
	return dir
}

// TestComposeUnionsMains: two executable charts (each importing the embedded
// claude target) compose by module import; their #Main namespaces union into one
// top-level states (both skills installed).
func TestComposeUnionsMains(t *testing.T) {
	aDir := writeChart(t, "coffeeenv.dev/test/a", `package env
import "coffeeenv.dev/lib/agent/claude"
#Main: {
	claude.#Main
	agent: skills: alpha: {body: "A"}
}
`)
	bDir := writeChart(t, "coffeeenv.dev/test/b", `package env
import "coffeeenv.dev/lib/agent/claude"
#Main: {
	claude.#Main
	agent: skills: beta: {body: "B"}
}
`)
	deps := map[string]string{"coffeeenv.dev/test/a": aDir, "coffeeenv.dev/test/b": bDir}
	r, err := Compose([]string{"coffeeenv.dev/test/a", "coffeeenv.dev/test/b"},
		deps, Opts{Engine: "global", Root: "~"}, nil, nil)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	m := byName(r.States)
	for _, want := range []string{"claude-code", "claude-skill-alpha", "claude-skill-beta"} {
		if _, ok := m[want]; !ok {
			t.Errorf("missing %q in composed states; got %v", want, names(r.States))
		}
	}
}

// TestComposeEmpty: a composition with no execs resolves to zero states.
func TestComposeEmpty(t *testing.T) {
	r, err := Compose(nil, nil, Opts{Engine: "global", Root: "~"}, nil, nil)
	if err != nil {
		t.Fatalf("compose empty: %v", err)
	}
	if len(r.States) != 0 {
		t.Errorf("empty composition should yield 0 states, got %v", names(r.States))
	}
}
