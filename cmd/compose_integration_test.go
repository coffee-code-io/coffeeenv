package cmd

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/coffee-code-io/coffeeenv/internal/chart"
	"github.com/coffee-code-io/coffeeenv/internal/venv"
)

func writeTmp(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsSub(s, sub string) bool { return strings.Contains(s, sub) }

func exampleDir(name string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "examples", name)
}

// pullExample fetches an example chart (local dir) into the active COFFEEENV_ROOT.
func pullExample(t *testing.T, name string) chart.Chart {
	t.Helper()
	c, err := chart.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := c.Pull(context.Background(), exampleDir(name)); err != nil {
		t.Fatalf("pull %s: %v", name, err)
	}
	return c
}

// TestApplyAccumulatesGlobal: applying a chart globally accumulates its module
// into the global manifest's execs and composes it into a plan.
func TestApplyAccumulatesGlobal(t *testing.T) {
	t.Setenv("COFFEEENV_ROOT", t.TempDir())
	pullExample(t, "claude-basic")

	tgt, err := resolveTarget(context.Background(), "claude-basic", "", "", nil)
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if len(tgt.manifest.Execs) != 1 || tgt.manifest.Execs[0] != "coffeeenv.dev/examples/claude-basic" {
		t.Fatalf("execs = %v, want [claude-basic module]", tgt.manifest.Execs)
	}

	p, resolved, err := computePlan(context.Background(), tgt, nil)
	if err != nil {
		t.Fatalf("computePlan: %v", err)
	}
	// The composed plan renders the claude target's files (claude-code itself may
	// already be installed on the host, so don't assert the npm install).
	var hasClaude bool
	for _, a := range p.Actions {
		if containsSub(a.Summary, ".claude") {
			hasClaude = true
		}
	}
	if !hasClaude {
		t.Errorf("composed plan should render claude files; actions=%v", p.Actions)
	}

	// Persist and re-read: the global manifest carries the exec.
	tgt.manifest.Values = resolved
	if err := writeGlobalManifest(tgt.manifest); err != nil {
		t.Fatal(err)
	}
	m, err := readGlobalManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Execs) != 1 {
		t.Errorf("persisted global execs = %v", m.Execs)
	}

	// Applying again is idempotent (dedup): execs stays length 1.
	tgt2, _ := resolveTarget(context.Background(), "claude-basic", "", "", nil)
	if len(tgt2.manifest.Execs) != 1 {
		t.Errorf("re-apply should dedup execs, got %v", tgt2.manifest.Execs)
	}
}

// TestVenvAccumulatesTwoCharts: two charts applied into a venv accumulate both
// execs and compose into the union (local engine -> venv prefix).
func TestVenvAccumulatesTwoCharts(t *testing.T) {
	t.Setenv("COFFEEENV_ROOT", t.TempDir())
	// Two library-ish executable charts that both build on claude but add
	// distinct skills (so the union is observable without agent-name conflicts).
	a := writeExecChart(t, "alpha", "coffeeenv.dev/test/alpha", `package env
import "coffeeenv.dev/lib/agent/claude"
#Main: {claude.#Main, agent: skills: alpha: {body: "A"}}
#Main
`)
	b := writeExecChart(t, "beta", "coffeeenv.dev/test/beta", `package env
import "coffeeenv.dev/lib/agent/claude"
#Main: {claude.#Main, agent: skills: beta: {body: "B"}}
#Main
`)
	_ = a
	_ = b

	v, _ := venv.Open("v")
	if err := v.Create(); err != nil {
		t.Fatal(err)
	}

	// Apply alpha, persist.
	tgt, err := resolveTarget(context.Background(), "alpha", "v", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	tgt.manifest.Values = map[string]string{}
	if err := tgt.save(tgt.manifest); err != nil {
		t.Fatal(err)
	}
	// Apply beta on top.
	tgt, err = resolveTarget(context.Background(), "beta", "v", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tgt.manifest.Execs) != 2 {
		t.Fatalf("venv execs = %v, want both", tgt.manifest.Execs)
	}
	p, _, err := computePlan(context.Background(), tgt, nil)
	if err != nil {
		t.Fatalf("computePlan: %v", err)
	}
	var skillStates int
	for _, a := range p.Actions {
		if a.Kind == "write-file" && (containsSub(a.Summary, "skills/alpha") || containsSub(a.Summary, "skills/beta")) {
			skillStates++
		}
	}
	if skillStates < 2 {
		t.Errorf("composed venv plan should install both skills; actions=%v", p.Actions)
	}
}

func writeExecChart(t *testing.T, name, module, env string) chart.Chart {
	t.Helper()
	c, err := chart.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeTmp(t, filepath.Join(dir, "cue.mod", "module.cue"), "module: \""+module+"\"\nlanguage: version: \"v0.9.0\"\n")
	writeTmp(t, filepath.Join(dir, "env.cue"), env)
	writeTmp(t, filepath.Join(dir, "manifest.json"), `{"module":"`+module+`","type":"executable"}`)
	if _, _, _, err := c.Pull(context.Background(), dir); err != nil {
		t.Fatalf("pull %s: %v", name, err)
	}
	return c
}
