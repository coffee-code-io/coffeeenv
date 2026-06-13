package cuelib

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestChooseDisjunction: a disjunction-typed @choose surfaces KindChoose with
// the disjunct values as options.
func TestChooseDisjunction(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "env.cue"), `package env
import st "coffeeenv.dev/lib/states"
mode: "fast" | "slow" @choose("Mode", order=1)
states: {s: st.#FileState & {path: "/tmp/x", content: mode}}
`)
	var gotKind InputKind
	var gotOpts []string
	prompt := func(in Input) (string, error) {
		gotKind, gotOpts = in.Kind, in.Options
		return "slow", nil
	}
	r, err := Resolve(dir, Opts{Engine: "global", Root: "~"}, nil, prompt)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotKind != KindChoose {
		t.Errorf("kind = %v, want choose", gotKind)
	}
	if strings.Join(gotOpts, ",") != "fast,slow" {
		t.Errorf("options = %v, want [fast slow]", gotOpts)
	}
	if got, _ := byName(r.States)["s"].Params["content"].(string); got != "slow" {
		t.Errorf("content = %q, want slow", got)
	}
}

// TestTextDisjunctionHint: a small disjunction on a plain @input stays KindText
// but carries the options (the CLI renders them inline as "(a/b)").
func TestTextDisjunctionHint(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "env.cue"), `package env
import st "coffeeenv.dev/lib/states"
mode: "fast" | "slow" @input("Mode", order=1)
states: {s: st.#FileState & {path: "/tmp/x", content: mode}}
`)
	var gotKind InputKind
	var gotOpts []string
	prompt := func(in Input) (string, error) {
		gotKind, gotOpts = in.Kind, in.Options
		return "fast", nil
	}
	if _, err := Resolve(dir, Opts{Engine: "global", Root: "~"}, nil, prompt); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotKind != KindText {
		t.Errorf("kind = %v, want text", gotKind)
	}
	if strings.Join(gotOpts, ",") != "fast,slow" {
		t.Errorf("options = %v, want [fast slow]", gotOpts)
	}
}

// TestMultichoiceList: @multichoice options come from a `from` registry's keys
// and the answer is injected as a CUE list.
func TestMultichoiceList(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "env.cue"), `package env
import (
	"strings"
	st "coffeeenv.dev/lib/states"
)
avail: {alpha: {}, beta: {}, gamma: {}}
picks: [...string] @multichoice("Pick", from=avail, order=1)
states: {s: st.#FileState & {path: "/tmp/x", content: strings.Join(picks, "|")}}
`)
	var gotKind InputKind
	var gotOpts []string
	prompt := func(in Input) (string, error) {
		gotKind, gotOpts = in.Kind, in.Options
		return "alpha,gamma", nil
	}
	r, err := Resolve(dir, Opts{Engine: "global", Root: "~"}, nil, prompt)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotKind != KindMultichoice {
		t.Errorf("kind = %v, want multichoice", gotKind)
	}
	sort.Strings(gotOpts)
	if strings.Join(gotOpts, ",") != "alpha,beta,gamma" {
		t.Errorf("options = %v, want registry keys", gotOpts)
	}
	if got, _ := byName(r.States)["s"].Params["content"].(string); got != "alpha|gamma" {
		t.Errorf("content = %q, want alpha|gamma (list joined)", got)
	}
}

// TestChooseFromDynamicAfterMap: a @choose whose `from` points at a @inputMap is
// processed after the map (higher order) and offers the entered keys.
func TestChooseFromDynamicAfterMap(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "env.cue"), `package env
import st "coffeeenv.dev/lib/states"
projects: {[string]: {repo: string @input("Repo", order=1)}} @inputMap("Project", order=1)
active: string @choose("Active", from=projects, order=2)
states: {s: st.#FileState & {path: "/tmp/x", content: active}}
`)
	keys := []string{"alpha", "beta"}
	ki, added := 0, 0
	var activeOpts []string
	prompt := func(in Input) (string, error) {
		switch {
		case strings.Contains(in.Prompt, "Project"):
			if ki < len(keys) {
				k := keys[ki]
				ki++
				return k, nil
			}
			return "", nil
		case strings.Contains(in.Prompt, "Repo"):
			return "/r", nil
		case strings.Contains(in.Prompt, "Add another"):
			added++
			if added < len(keys) {
				return "y", nil
			}
			return "n", nil
		case strings.Contains(in.Prompt, "Active"):
			activeOpts = in.Options
			return "beta", nil
		}
		return "", nil
	}
	r, err := Resolve(dir, Opts{Engine: "global", Root: "~"}, nil, prompt)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	sort.Strings(activeOpts)
	if strings.Join(activeOpts, ",") != "alpha,beta" {
		t.Errorf("active options = %v, want the entered projects", activeOpts)
	}
	if got, _ := byName(r.States)["s"].Params["content"].(string); got != "beta" {
		t.Errorf("active = %q, want beta", got)
	}
}

// TestChooseUnquotesKeys: keys that need quoting (e.g. "my-repo") are offered
// and stored unquoted — the quotes must not leak into the chosen value.
func TestChooseUnquotesKeys(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "env.cue"), `package env
import st "coffeeenv.dev/lib/states"
avail: {"my-repo": {}, plain: {}}
active: string @choose("Active", from=avail, order=1)
states: {s: st.#FileState & {path: "/tmp/x", content: active}}
`)
	var gotOpts []string
	prompt := func(in Input) (string, error) {
		gotOpts = in.Options
		return "my-repo", nil
	}
	r, err := Resolve(dir, Opts{Engine: "global", Root: "~"}, nil, prompt)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	for _, o := range gotOpts {
		if strings.ContainsAny(o, `"`) {
			t.Errorf("option %q is quoted; keys should be unquoted", o)
		}
	}
	if got, _ := byName(r.States)["s"].Params["content"].(string); got != "my-repo" {
		t.Errorf("active = %q, want my-repo (no quotes)", got)
	}
}

// TestChooseEmptyOptionsSkipped: a @choose with no options resolves to empty
// (not prompted) so the field stays concrete.
func TestChooseEmptyOptionsSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "env.cue"), `package env
import st "coffeeenv.dev/lib/states"
avail: {}
active: string @choose("Active", from=avail, order=1)
states: {s: st.#FileState & {path: "/tmp/x", content: "active=\(active)"}}
`)
	called := false
	prompt := func(in Input) (string, error) {
		called = true
		return "x", nil
	}
	r, err := Resolve(dir, Opts{Engine: "global", Root: "~"}, nil, prompt)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if called {
		t.Errorf("empty-option choose should not prompt")
	}
	if got, _ := byName(r.States)["s"].Params["content"].(string); got != "active=" {
		t.Errorf("content = %q, want active= (empty)", got)
	}
}
