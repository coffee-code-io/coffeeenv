package cuelib

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestInputMapExistingEntriesFirst: pre-seeded map entries are processed before
// any new-key prompt. A complete entry (alpha) is not re-prompted and gets an
// "already set up" notice; a partial one (beta) has only its missing field
// requested; then, the map being open, an "Add another?" confirm is offered.
func TestInputMapExistingEntriesFirst(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "env.cue"), `package env
import st "coffeeenv.dev/lib/states"
projects: {[string]: {repo: string @input("Repo", order=1), lang: string @input("Lang", order=2)}} @inputMap("Project", order=1)
states: {
	for k, p in projects {
		"s-\(k)": st.#FileState & {path: "/tmp/\(k)", content: "\(p.repo):\(p.lang)"}
	}
}
`)
	given := map[string]string{
		"projects.alpha.repo": "/a",
		"projects.alpha.lang": "go",
		"projects.beta.repo":  "/b", // beta.lang missing -> must be prompted
	}

	var notices []string
	var promptedFields []string
	askedAddMore := false
	prompt := func(in Input) (string, error) {
		switch in.Kind {
		case KindNotice:
			notices = append(notices, in.Prompt)
			return "", nil
		case KindConfirm:
			if strings.Contains(in.Prompt, "Add another") {
				askedAddMore = true
			}
			return "n", nil // don't add new entries
		default:
			promptedFields = append(promptedFields, in.Prompt)
			if strings.Contains(in.Prompt, "Lang") {
				return "python", nil
			}
			return "/x", nil
		}
	}

	r, err := Resolve(dir, Opts{Engine: "global", Root: "~"}, given, prompt)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// alpha was complete: no Repo/Lang prompts for it; beta.lang was prompted.
	for _, p := range promptedFields {
		if strings.Contains(p, "Repo") {
			t.Errorf("no repo should be prompted (alpha given, beta given); got %q", p)
		}
	}
	if len(promptedFields) != 1 || !strings.Contains(promptedFields[0], "Lang") {
		t.Errorf("only beta.lang should be prompted; got %v", promptedFields)
	}
	if !askedAddMore {
		t.Errorf("an open map with existing entries should offer to add more")
	}
	// alpha's notice is the "already set up" form; beta's is the configuring form.
	joined := strings.Join(notices, "\n")
	if !strings.Contains(joined, `"alpha" — already set up`) {
		t.Errorf("expected alpha already-set-up notice; got %v", notices)
	}
	if !strings.Contains(joined, `Configuring projects "beta"`) {
		t.Errorf("expected beta configuring notice; got %v", notices)
	}

	m := byName(r.States)
	if got, _ := m["s-alpha"].Params["content"].(string); got != "/a:go" {
		t.Errorf("alpha content = %q", got)
	}
	if got, _ := m["s-beta"].Params["content"].(string); got != "/b:python" {
		t.Errorf("beta content = %q", got)
	}
}

// TestInputMapClosedNoAddMore: a closed @inputMap (no `[string]:` pattern) gets
// its existing entries resolved but no add-more prompt.
func TestInputMapClosedNoAddMore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "env.cue"), `package env
import st "coffeeenv.dev/lib/states"
regions: close({us: {zone: string @input("Zone", order=1)}}) @inputMap("Region", order=1)
states: {s: st.#FileState & {path: "/tmp/x", content: regions.us.zone}}
`)
	given := map[string]string{"regions.us.zone": "z1"}

	askedAddMore := false
	prompt := func(in Input) (string, error) {
		if in.Kind == KindConfirm && strings.Contains(in.Prompt, "Add another") {
			askedAddMore = true
		}
		return "", nil
	}
	r, err := Resolve(dir, Opts{Engine: "global", Root: "~"}, given, prompt)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if askedAddMore {
		t.Errorf("a closed map should not offer to add more entries")
	}
	if got, _ := byName(r.States)["s"].Params["content"].(string); got != "z1" {
		t.Errorf("content = %q, want z1", got)
	}
}

// TestChooseNoneOption: a @choose with none= offers the opt-out first and
// resolves to "" when picked, or the chosen key otherwise.
func TestChooseNoneOption(t *testing.T) {
	chart := `package env
import st "coffeeenv.dev/lib/states"
reg: {ts: {}, go: {}}
lang: string @choose("Lang", from=reg, none="(none)", order=1)
states: {s: st.#FileState & {path: "/tmp/x", content: "lang=\(lang)"}}
`
	run := func(pick string) (string, []string) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "env.cue"), chart)
		var opts []string
		prompt := func(in Input) (string, error) {
			if in.Kind == KindChoose {
				opts = in.Options
				return pick, nil
			}
			return "", nil
		}
		r, err := Resolve(dir, Opts{Engine: "global", Root: "~"}, nil, prompt)
		if err != nil {
			t.Fatalf("resolve (pick %q): %v", pick, err)
		}
		got, _ := byName(r.States)["s"].Params["content"].(string)
		return got, opts
	}

	if got, opts := run("(none)"); got != "lang=" {
		t.Errorf("picking none: content = %q, want empty lang; opts=%v", got, opts)
	} else if len(opts) == 0 || opts[0] != "(none)" {
		t.Errorf("none option should be offered first; opts=%v", opts)
	}
	if got, _ := run("ts"); got != "lang=ts" {
		t.Errorf("picking ts: content = %q", got)
	}
}
