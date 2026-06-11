package cuelib

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupPrompt answers the coffeectx.#Setup scalar inputs and drives the
// @inputMap project loop: it yields each name in `names` in turn (then "" to
// finish) and answers "Add another?" yes until the list is exhausted.
func setupPrompt(names []string) PromptFunc {
	ni, added := 0, 0
	return func(in Input) (string, error) {
		switch p := in.Prompt; {
		case strings.Contains(p, "Install coffeectx"):
			return "true", nil
		case strings.Contains(p, "Auth type"):
			return "apiKey", nil
		case strings.Contains(p, "base URL"):
			return "https://api.example.com", nil
		case strings.Contains(p, "API key"):
			return "sk", nil
		case strings.Contains(p, "Embeddings"):
			return "e", nil
		case strings.Contains(p, "Indexer"):
			return "i", nil
		case strings.Contains(p, "UI agent"):
			return "u", nil
		case strings.Contains(p, "Auto-launch"):
			return "false", nil
		case strings.Contains(p, "Project name"):
			if ni < len(names) {
				n := names[ni]
				ni++
				return n, nil
			}
			return "", nil // finish
		case strings.Contains(p, "Repo path"):
			return "/r", nil
		case strings.Contains(p, "Language"):
			return "go", nil
		case strings.Contains(p, "Add another"):
			added++
			if added < len(names) {
				return "y", nil
			}
			return "n", nil
		}
		return "", nil
	}
}

func setupProjects(t *testing.T, names []string) map[string]any {
	t.Helper()
	r, err := Resolve(exampleDir("coffeectx-setup"), Opts{Engine: "global", Root: "~", OS: "darwin"}, nil, setupPrompt(names))
	if err != nil {
		t.Fatalf("resolve %v: %v", names, err)
	}
	data, _ := byName(r.States)["coffeecode-config"].Params["data"].(map[string]any)
	pj, _ := data["projects"].(map[string]any)
	return pj
}

// TestSetupInputMapProjects: the interactive @inputMap project loop yields a
// concrete config for zero, one, and two projects. Zero is the regression guard
// for the empty-comprehension-in-open-`data` taint (fixed by the `{}` seed).
func TestSetupInputMapProjects(t *testing.T) {
	if p := setupProjects(t, nil); len(p) != 0 {
		t.Errorf("zero: want 0 projects, got %v", p)
	}
	if p := setupProjects(t, []string{"alpha"}); len(p) != 1 || p["alpha"] == nil {
		t.Errorf("one: want alpha, got %v", p)
	}
	p := setupProjects(t, []string{"alpha", "beta"})
	if len(p) != 2 || p["alpha"] == nil || p["beta"] == nil {
		t.Errorf("two: want alpha+beta, got %v", p)
	}
}

// TestStateOrder: the states map is flattened by (order, key) — npm (25) before
// file (50) before env (60) — independent of map iteration order.
func TestStateOrder(t *testing.T) {
	raws, err := EvalStates(exampleDir("claude-basic"), Opts{Engine: "local", Root: "/tmp/v"})
	if err != nil {
		t.Fatalf("%v", err)
	}
	pos := map[string]int{}
	for i, r := range raws {
		pos[r.Name] = i
	}
	if pos["claude-code"] > pos["claude-claudemd"] {
		t.Errorf("npm(25) should precede file(50): %v", names(raws))
	}
	if pos["claude-claudemd"] > pos["CLAUDE_CONFIG_DIR"] {
		t.Errorf("file(50) should precede env(60): %v", names(raws))
	}
}

// TestSkillFilesCopy: a skill sourced from a filesystem path becomes a copy
// state whose src is resolved absolute against the chart directory.
func TestSkillFilesCopy(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "env.cue"), `package env
import "coffeeenv.dev/lib/agent/claude"
claude.#Claude
agent: skills: docs: {files: "./skilldir"}
`)
	if err := os.MkdirAll(filepath.Join(dir, "skilldir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "skilldir", "SKILL.md"), "hi")

	raws, err := EvalStates(dir, Opts{Engine: "global", Root: "~"})
	if err != nil {
		t.Fatalf("%v", err)
	}
	cp, ok := byName(raws)["claude-skill-docs-files"]
	if !ok {
		t.Fatalf("missing copy state; got %v", names(raws))
	}
	if cp.Type != "copy" {
		t.Errorf("type = %s, want copy", cp.Type)
	}
	if src, _ := cp.Params["src"].(string); !filepath.IsAbs(src) || !strings.HasSuffix(src, "skilldir") {
		t.Errorf("src should be absolute under the chart dir, got %q", src)
	}
	if dst, _ := cp.Params["dst"].(string); dst != "~/.claude/skills/docs" {
		t.Errorf("dst = %q", dst)
	}
}
