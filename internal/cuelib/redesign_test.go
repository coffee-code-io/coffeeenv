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
		case strings.Contains(p, "Auth type"):
			return "apiKey", nil
		case strings.Contains(p, "Provider alias"):
			return "", nil // empty -> custom url path
		case strings.Contains(p, "base URL"):
			return "https://api.example.com", nil
		case strings.Contains(p, "API key"):
			return "sk", nil
		case strings.Contains(p, "Active project"):
			return "", nil
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

// TestSetupOAuthEmbeddings: in openai-oauth mode the UI/job auth carries only
// authType (no apiKey), while the embeddings auth is a separate apiKey credential
// prompted independently (oauth can't embed).
func TestSetupOAuthEmbeddings(t *testing.T) {
	names := []string{"alpha"}
	ni := 0
	prompt := func(in Input) (string, error) {
		switch p := in.Prompt; {
		case strings.Contains(p, "Auth type"):
			return "openai-oauth", nil
		case strings.Contains(p, "Embeddings provider"):
			return "", nil
		case strings.Contains(p, "Embeddings custom"):
			return "https://embed.example.com", nil
		case strings.Contains(p, "Embeddings API key"):
			return "embed-key", nil
		case strings.Contains(p, "Embeddings model"):
			return "e", nil
		case strings.Contains(p, "Indexer"):
			return "i", nil
		case strings.Contains(p, "UI agent"):
			return "u", nil
		case strings.Contains(p, "Active project"):
			return "", nil
		case strings.Contains(p, "Auto-launch"):
			return "false", nil
		case strings.Contains(p, "Project name"):
			if ni < len(names) {
				n := names[ni]
				ni++
				return n, nil
			}
			return "", nil
		case strings.Contains(p, "Repo path"):
			return "/r", nil
		case strings.Contains(p, "Language"):
			return "go", nil
		case strings.Contains(p, "Add another"):
			return "n", nil
		}
		return "", nil
	}
	r, err := Resolve(exampleDir("coffeectx-setup"), Opts{Engine: "global", Root: "~", OS: "darwin"}, nil, prompt)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	data, _ := byName(r.States)["coffeecode-config"].Params["data"].(map[string]any)
	p, _ := data["projects"].(map[string]any)["alpha"].(map[string]any)

	agentAuth, _ := p["agent"].(map[string]any)["auth"].(map[string]any)
	if got, _ := agentAuth["authType"].(string); got != "openai-oauth" {
		t.Errorf("UI auth.authType = %v, want openai-oauth", got)
	}
	if _, present := agentAuth["apiKey"]; present {
		t.Errorf("oauth UI auth should carry no apiKey, got %v", agentAuth)
	}
	embedAuth, _ := p["core"].(map[string]any)["embed"].(map[string]any)["auth"].(map[string]any)
	if got, _ := embedAuth["authType"].(string); got != "apiKey" {
		t.Errorf("embed auth.authType = %v, want apiKey", got)
	}
	if got, _ := embedAuth["apiKey"].(string); got != "embed-key" {
		t.Errorf("embed auth.apiKey = %v, want the separate embeddings key", got)
	}
	if got, _ := embedAuth["url"].(string); got != "https://embed.example.com" {
		t.Errorf("embed auth.url = %v", got)
	}
}

// TestLspRegistry: lsp.available works as a registry independent of coffeectx —
// #Setup registers built-ins, #InstallLsp lifts selected install states, and a
// user-added language is installable.
func TestLspRegistry(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "env.cue"), `package env
import lsplib "coffeeenv.dev/lib/lsp"
lsplib.#Setup
lsplib.#Main & {languages: ["go", "custom"]}
lsp: available: custom: {command: "customls", installState: {type: "shell", run: "install-custom"}}
`)
	raws, err := EvalStates(dir, Opts{Engine: "global", Root: "~"})
	if err != nil {
		t.Fatalf("%v", err)
	}
	m := byName(raws)
	goLsp, ok := m["lsp-install-go"]
	if !ok {
		t.Fatalf("missing lsp-install-go; got %v", names(raws))
	}
	if goLsp.Type != "shell" {
		t.Errorf("go install state type = %s, want shell", goLsp.Type)
	}
	if _, ok := m["lsp-install-custom"]; !ok {
		t.Errorf("user-registered lsp should be installable; got %v", names(raws))
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
claude.#Main
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

func TestCopyStateFromDependencyUsesDependencyDir(t *testing.T) {
	root := t.TempDir()
	mainDir := filepath.Join(root, "main")
	depDir := filepath.Join(root, "dep")
	if err := os.MkdirAll(mainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(depDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(mainDir, "env.cue"), `package env
import dep "example.com/dep"
dep.#Main
`)
	writeFile(t, filepath.Join(depDir, "env.cue"), `package dep
import "coffeeenv.dev/lib/agent/pi"
#Main: {
	pi.#Main
	agent: extensions: helper: {files: "files/helper"}
}
`)
	if err := os.MkdirAll(filepath.Join(depDir, "files", "helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(depDir, "files", "helper", "main.py"), "hi")

	raws, err := EvalStates(mainDir, Opts{
		Engine: "global",
		Root:   "~",
		Deps:   map[string]string{"example.com/dep": depDir},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	cp, ok := byName(raws)["pi-extension-helper-files"]
	if !ok {
		t.Fatalf("missing copy state; got %v", names(raws))
	}
	want := filepath.Join(depDir, "files", "helper")
	if src, _ := cp.Params["src"].(string); src != want {
		t.Errorf("src = %q, want %q", src, want)
	}
}
