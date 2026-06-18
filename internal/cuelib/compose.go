package cuelib

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Compose synthesizes a composition package that imports each exec module and
// unions its `#Main` at the package root (so their shared namespaces merge into
// one top-level `states`), mounts the given deps (module path -> chart dir),
// injects `given` values, and resolves it. An empty exec list yields an empty
// (concrete) states map. skills maps a skill name to its directory; each is added
// to `agent.skills` as a file-backed #Skill that the active agent target renders.
func Compose(execs []string, skills map[string]string, deps map[string]string, opts Opts, given map[string]string, prompt PromptFunc) (Result, error) {
	dir, err := os.MkdirTemp("", "coffeeenv-compose-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(dir)

	if err := os.MkdirAll(filepath.Join(dir, "cue.mod"), 0o755); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "cue.mod", "module.cue"),
		[]byte("module: \"coffeeenv.dev/composition\"\nlanguage: version: \"v0.9.0\"\n"), 0o644); err != nil {
		return Result{}, err
	}

	var b strings.Builder
	b.WriteString("package env\n\n")
	b.WriteString("import st \"coffeeenv.dev/lib/states\"\n")
	if len(skills) > 0 {
		b.WriteString("import ag \"coffeeenv.dev/lib/agent\"\n")
	}
	for i, m := range execs {
		// Charts use `package env`; import by module path with that qualifier.
		fmt.Fprintf(&b, "import e%d %q\n", i, m+":env")
	}
	b.WriteString("\n// Empty by default; each exec's #Main contributes entries.\n")
	b.WriteString("states: {[string]: st.#State}\n")
	for i := range execs {
		fmt.Fprintf(&b, "e%d.#Main\n", i)
	}
	// Skills: file-backed #Skills added to the agent namespace; the active agent
	// target (from an exec's #Main) renders them. Sorted for deterministic output.
	if len(skills) > 0 {
		b.WriteString("agent: ag.#NS\n")
		names := make([]string, 0, len(skills))
		for name := range skills {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(&b, "agent: skills: %q: {files: %q}\n", name, skills[name])
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "env.cue"), []byte(b.String()), 0o644); err != nil {
		return Result{}, err
	}

	o := opts
	o.Deps = deps
	return Resolve(dir, o, given, prompt)
}
