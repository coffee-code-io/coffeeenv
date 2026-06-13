// Package cuelib loads a chart's user CUE files, unifies them with the bundled
// CUE library (embedded and mounted as an importable module), injects the engine
// context, evaluates, and extracts the flat `states` list for the execution
// layer.
package cuelib

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"

	"github.com/coffee-code-io/coffeeenv/internal/state"
)

// userModule is the module path synthesized for the chart when it doesn't ship
// its own cue.mod/module.cue.
const userModule = "coffeeenv.dev/user"

// Opts carries the engine context injected into CUE.
type Opts struct {
	Engine string            // "global" | "local"
	Root   string            // "~" for global; the venv dir for local
	OS     string            // host GOOS ("darwin", "linux", ...); empty defaults to runtime.GOOS
	Deps   map[string]string // module path -> chart dir; mounted so `import "<module>"` resolves
}

// buildBase loads the chart's *.cue from chartDir, overlays the embedded library
// so `import "coffeeenv.dev/lib/..."` resolves, injects the engine context, and
// builds the (possibly non-concrete) value. extra maps a filename to CUE source
// overlaid into the chart package (used by Resolve to inject resolved input
// values at the SOURCE level so struct comprehensions re-trigger). It returns
// the chart's package name. Non-concreteness is NOT an error here.
func buildBase(chartDir string, opts Opts, extra map[string]string) (*cue.Context, cue.Value, string, error) {
	venvAbs, err := filepath.Abs(chartDir)
	if err != nil {
		return nil, cue.Value{}, "", err
	}

	overlay := map[string]load.Source{}
	if err := mountEmbed(overlay, venvAbs); err != nil {
		return nil, cue.Value{}, "", fmt.Errorf("mount cue library: %w", err)
	}
	if err := mountDeps(overlay, venvAbs, opts.Deps); err != nil {
		return nil, cue.Value{}, "", fmt.Errorf("mount deps: %w", err)
	}
	ensureUserModule(overlay, venvAbs)
	injectContext(overlay, venvAbs, opts)
	for name, src := range extra {
		overlay[filepath.Join(venvAbs, name)] = load.FromString(src)
	}

	cfg := &load.Config{Dir: venvAbs, Overlay: overlay}
	insts := load.Instances([]string{"."}, cfg)
	if len(insts) == 0 {
		return nil, cue.Value{}, "", fmt.Errorf("no CUE instances found in %s", venvAbs)
	}
	inst := insts[0]
	if inst.Err != nil {
		return nil, cue.Value{}, "", fmt.Errorf("load CUE: %w", inst.Err)
	}

	ctx := cuecontext.New()
	v := ctx.BuildInstance(inst)
	// Deliberately NOT v.Err() here. The base is built before inputs are
	// resolved, so the top-level value is expected to be incomplete — and
	// v.Err() returns the error a value *represents*, which for a pending
	// top-level comprehension (e.g. `if <unresolved input> {…}` writing into a
	// namespace) is an "incomplete" error even though nothing is actually wrong.
	// v.Validate() (no Concrete) reports genuine conflicts recursively while
	// tolerating incompleteness, which is what we want until the final concrete
	// check after resolution.
	if err := v.Validate(); err != nil {
		return nil, cue.Value{}, "", fmt.Errorf("evaluate CUE: %w", err)
	}
	return ctx, v, inst.PkgName, nil
}

// EvalStates is a convenience wrapper for charts with no unresolved inputs.
func EvalStates(chartDir string, opts Opts) ([]state.RawState, error) {
	r, err := Resolve(chartDir, opts, nil, nil)
	if err != nil {
		return nil, err
	}
	return r.States, nil
}

// mountEmbed walks the embedded lib/ tree and overlays every package file under
// <venv>/cue.mod/pkg/coffeeenv.dev/lib/... — the location CUE resolves external
// imports from. The library's own cue.mod/ is skipped.
func mountEmbed(overlay map[string]load.Source, venvAbs string) error {
	pkgRoot := filepath.Join(venvAbs, "cue.mod", "pkg", filepath.FromSlash(libModule))
	return fs.WalkDir(libFS, "lib", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, "lib/")
		if strings.HasPrefix(rel, "cue.mod/") {
			return nil // skip the library's own module marker
		}
		data, err := libFS.ReadFile(p)
		if err != nil {
			return err
		}
		overlay[filepath.Join(pkgRoot, filepath.FromSlash(rel))] = load.FromBytes(data)
		return nil
	})
}

// mountDeps overlays each dependency chart's files under
// <venv>/cue.mod/pkg/<module>/… so the composition can `import "<module>"`. A
// dep's own cue.mod/ and excluded dirs (.git, node_modules) are skipped; they
// share the composition's cue.mod/pkg tree, so a dep importing another dep (or
// the embedded lib) resolves.
func mountDeps(overlay map[string]load.Source, venvAbs string, deps map[string]string) error {
	for module, dir := range deps {
		pkgRoot := filepath.Join(venvAbs, "cue.mod", "pkg", filepath.FromSlash(module))
		root := dir
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			if d.IsDir() {
				if p != root && (d.Name() == "cue.mod" || excludedDir(d.Name())) {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".cue") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			overlay[filepath.Join(pkgRoot, filepath.FromSlash(rel))] = load.FromBytes(data)
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func excludedDir(name string) bool { return name == ".git" || name == "node_modules" }

// injectContext overlays a concrete context/_inject.cue into the mounted
// library so library helpers see the active engine/root. This unifies with the
// embedded context schema and avoids relying on CUE tag injection reaching
// imported packages.
func injectContext(overlay map[string]load.Source, venvAbs string, opts Opts) {
	if opts.Engine == "" {
		return
	}
	root := opts.Root
	if root == "" {
		root = "~"
	}
	goos := opts.OS
	if goos == "" {
		goos = runtime.GOOS
	}
	pkgRoot := filepath.Join(venvAbs, "cue.mod", "pkg", filepath.FromSlash(libModule))
	// NB: CUE's loader ignores files beginning with "_" or ".", so the injected
	// file must not start with an underscore.
	path := filepath.Join(pkgRoot, "context", "inject.cue")
	src := fmt.Sprintf("package context\nengine: %q\nroot: %q\nos: %q\n", opts.Engine, root, goos)
	overlay[path] = load.FromString(src)
}

// ensureUserModule overlays a minimal cue.mod/module.cue for the chart if one is
// not already present, so the chart counts as a module and imports resolve.
func ensureUserModule(overlay map[string]load.Source, venvAbs string) {
	modPath := filepath.Join(venvAbs, "cue.mod", "module.cue")
	if _, err := os.Stat(modPath); err == nil {
		return // exists on disk; respect it
	}
	overlay[modPath] = load.FromString(fmt.Sprintf("module: %q\nlanguage: version: \"v0.9.0\"\n", userModule))
}

// decodeRawStates decodes the `states` field into a flat ordered RawState list.
// The canonical shape is a map keyed by name (the map key IS the state's name),
// flattened sorted by (order, key). A bare list is also accepted for low-level
// charts, preserving authored order; such states are unnamed. `type` and `order`
// are stripped from the param bag.
func decodeRawStates(statesV cue.Value) ([]state.RawState, error) {
	switch statesV.IncompleteKind() {
	case cue.ListKind:
		var maps []map[string]any
		if err := statesV.Decode(&maps); err != nil {
			return nil, fmt.Errorf("decode states list: %w", err)
		}
		out := make([]state.RawState, 0, len(maps))
		for i, m := range maps {
			rs, err := rawFromMap(m, "")
			if err != nil {
				return nil, fmt.Errorf("states[%d]: %w", i, err)
			}
			out = append(out, rs)
		}
		return out, nil
	case cue.StructKind:
		var byKey map[string]map[string]any
		if err := statesV.Decode(&byKey); err != nil {
			return nil, fmt.Errorf("decode states map: %w", err)
		}
		type entry struct {
			key   string
			order int
			rs    state.RawState
		}
		entries := make([]entry, 0, len(byKey))
		for key, m := range byKey {
			rs, err := rawFromMap(m, key)
			if err != nil {
				return nil, fmt.Errorf("states[%q]: %w", key, err)
			}
			entries = append(entries, entry{key: key, order: orderOf(m), rs: rs})
		}
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].order != entries[j].order {
				return entries[i].order < entries[j].order
			}
			return entries[i].key < entries[j].key
		})
		out := make([]state.RawState, len(entries))
		for i, e := range entries {
			out[i] = e.rs
		}
		return out, nil
	default:
		return nil, fmt.Errorf("states must be a list or a map, got %v", statesV.IncompleteKind())
	}
}

// rawFromMap splits a decoded state object into a typed/name/params triple,
// dropping the meta fields `type` and `order` from the param bag.
func rawFromMap(m map[string]any, name string) (state.RawState, error) {
	typ, _ := m["type"].(string)
	if typ == "" {
		return state.RawState{}, fmt.Errorf("missing `type`")
	}
	params := make(map[string]any, len(m))
	for k, val := range m {
		if k == "type" || k == "order" {
			continue
		}
		params[k] = val
	}
	return state.RawState{Type: typ, Name: name, Params: params}, nil
}

// orderOf reads the numeric `order` meta field (defaulting large when absent).
func orderOf(m map[string]any) int {
	switch v := m["order"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 1 << 30
}
