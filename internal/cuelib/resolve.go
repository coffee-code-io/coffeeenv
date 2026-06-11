package cuelib

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"

	"github.com/coffee-code-io/coffeeenv/internal/state"
)

// Input is a chart field marked promptable with @input("prompt", order=N). Name
// is the field's CUE path (e.g. "projects[0].lspCommand") used for display and
// as the key in the values map; Path is the resolved selector path for filling.
type Input struct {
	Name   string
	Path   cue.Path
	Prompt string
	Order  int
}

// PromptFunc resolves an input value interactively. A nil PromptFunc means
// non-interactive: unresolved inputs become an error.
type PromptFunc func(Input) (string, error)

// Result is the outcome of resolution: the final values map (for the manifest)
// and the decoded flat states list.
type Result struct {
	Values map[string]string
	States []state.RawState
}

// leaf is a scalar field discovered during the scan.
type leaf struct {
	name string
	path cue.Path
}

// DynMap is a keyed-map field annotated @inputMap("<key prompt>", order=N) whose
// entries are supplied interactively: the resolver prompts for a key, resolves
// that entry's own @input fields, then asks whether to add another. The entry
// schema is the map's pattern constraint (e.g. {[string]: #Project}).
//
// A map that ends up empty is injected as `{}` so downstream stays concrete.
// (Note: a comprehension over an empty map still reads as incomplete once it's
// embedded in an open struct — charts that fold it into such a field should seed
// that field with a literal `{}`, see coffeectx #Setup.)
type DynMap struct {
	Name   string
	Path   cue.Path
	Prompt string
	Order  int
}

// Resolve builds the chart, fills in the given values, and iteratively resolves
// remaining @input fields at any depth (re-rendering after each so CUE
// propagation can fix dependents). A non-annotated non-fixed scalar leaf is an
// error.
func Resolve(chartDir string, opts Opts, given map[string]string, prompt PromptFunc) (Result, error) {
	ctx, base, pkg, err := buildBase(chartDir, opts, nil)
	if err != nil {
		return Result{}, err
	}

	inputs, leaves, dynMaps, err := scanInputs(base)
	if err != nil {
		return Result{}, err
	}
	pathOf := map[string]cue.Path{}
	for _, lf := range leaves {
		pathOf[lf.name] = lf.path
	}

	values := map[string]string{}
	for k, v := range given {
		values[k] = v
	}

	// Phase 1: resolve the statically-known scalar @input fields.
	if err := resolveScalars(ctx, base, inputs, leaves, pathOf, values, prompt); err != nil {
		return Result{}, err
	}

	// Phase 2: resolve user-keyed maps (@inputMap). Each adds entries and their
	// nested @input fields to `values`. Order across maps is stable.
	sort.Slice(dynMaps, func(i, j int) bool {
		if dynMaps[i].Order != dynMaps[j].Order {
			return dynMaps[i].Order < dynMaps[j].Order
		}
		return dynMaps[i].Name < dynMaps[j].Name
	})
	for _, dm := range dynMaps {
		if err := resolveDynamicMap(ctx, base, dm, pathOf, values, prompt); err != nil {
			return Result{}, err
		}
	}

	// Phase 3: re-build the chart with the resolved values injected as a SOURCE
	// overlay (not FillPath/Unify) so struct comprehensions with computed labels
	// re-evaluate against concrete data, then extract `states`.
	final, err := finalValue(ctx, chartDir, opts, pkg, values, pathOf, dynMaps)
	if err != nil {
		return Result{}, err
	}
	statesV := final.LookupPath(cue.ParsePath("states"))
	if !statesV.Exists() {
		return Result{}, fmt.Errorf("chart has no top-level `states` field")
	}
	if err := statesV.Validate(cue.Concrete(true)); err != nil {
		return Result{}, fmt.Errorf("`states` is not concrete: %w", err)
	}
	raws, err := decodeRawStates(statesV)
	if err != nil {
		return Result{}, err
	}
	resolveCopySrc(raws, chartDir)
	return Result{Values: values, States: raws}, nil
}

// resolveCopySrc rewrites each copy state's relative `src` to an absolute path
// anchored at the chart directory, so a `files: "./skills/x"` in a chart
// resolves regardless of the apply layer's working directory. Paths that are
// already absolute or start with ~ are left untouched.
func resolveCopySrc(raws []state.RawState, chartDir string) {
	base, err := filepath.Abs(chartDir)
	if err != nil {
		return
	}
	for i := range raws {
		if raws[i].Type != "copy" {
			continue
		}
		src, _ := raws[i].Params["src"].(string)
		if src == "" || filepath.IsAbs(src) || strings.HasPrefix(src, "~") {
			continue
		}
		raws[i].Params["src"] = filepath.Join(base, src)
	}
}

// resolveScalars drives the iterative prompt loop over the statically-discovered
// scalar @input leaves: fill known values, find the next non-concrete @input,
// prompt it, repeat. A non-@input non-concrete leaf is an error.
func resolveScalars(ctx *cue.Context, base cue.Value, inputs map[string]Input, leaves []leaf, pathOf map[string]cue.Path, values map[string]string, prompt PromptFunc) error {
	for {
		cur, err := fillValues(ctx, base, values, pathOf)
		if err != nil {
			return err
		}

		var promptable []Input
		var badGiven, plain []string
		for _, lf := range leaves {
			if leafFixed(cur, lf.path) {
				continue
			}
			if in, ok := inputs[lf.name]; ok {
				if _, given := values[lf.name]; given {
					badGiven = append(badGiven, lf.name)
				} else {
					promptable = append(promptable, in)
				}
			} else {
				plain = append(plain, lf.name)
			}
		}

		if len(promptable) > 0 {
			if prompt == nil {
				return missingInputsErr(promptable, plain)
			}
			next := leastInput(promptable)
			val, err := prompt(next)
			if err != nil {
				return err
			}
			values[next.Name] = val
			continue
		}

		if len(badGiven) > 0 || len(plain) > 0 {
			return unresolvedErr(cur, pathOf, badGiven, plain)
		}
		return nil
	}
}

// resolveDynamicMap interactively grows a @inputMap field: prompt for a key,
// resolve that entry's nested inputs, then ask whether to add another. With a
// nil PromptFunc (non-interactive) keys come from `given`/--value, so there is
// nothing to do here.
func resolveDynamicMap(ctx *cue.Context, base cue.Value, dm DynMap, pathOf map[string]cue.Path, values map[string]string, prompt PromptFunc) error {
	if prompt == nil {
		return nil
	}
	label := leafLabel(dm.Name)
	for {
		key, err := prompt(Input{Name: dm.Name, Prompt: fmt.Sprintf("%s (leave empty to finish):", dm.Prompt)})
		if err != nil {
			return err
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil
		}
		entry := cue.MakePath(append(dm.Path.Selectors(), cue.Str(key))...)
		// Resolve the entry's own @input fields. The map key materializes from the
		// entry's filled sub-fields.
		if err := resolveEntry(ctx, base, entry, pathOf, values, prompt); err != nil {
			return err
		}
		ans, err := prompt(Input{Name: dm.Name, Prompt: fmt.Sprintf("Add another %s? (y/N):", label)})
		if err != nil {
			return err
		}
		if !isYes(ans) {
			return nil
		}
	}
}

// resolveEntry resolves every @input field (and any nested @inputMap) under a
// single map entry at `entry`. The value schema is the map's pattern constraint,
// so we materialize the key with an empty struct to make the schema visible,
// then prompt its scalars the same way resolveScalars does.
func resolveEntry(ctx *cue.Context, base cue.Value, entry cue.Path, pathOf map[string]cue.Path, values map[string]string, prompt PromptFunc) error {
	for {
		cur, err := fillValues(ctx, base, values, pathOf)
		if err != nil {
			return err
		}
		probe := cur.FillPath(entry, ctx.CompileString("{}"))
		elem := probe.LookupPath(entry)
		if !elem.Exists() {
			return fmt.Errorf("cannot resolve map entry %q", entry.String())
		}

		inputs := map[string]Input{}
		var leaves []leaf
		var nested []DynMap
		scanValue(elem, entry.Selectors(), inputs, &leaves, &nested)

		var promptable []Input
		for _, lf := range leaves {
			if leafFixed(probe, lf.path) {
				continue
			}
			if in, ok := inputs[lf.name]; ok {
				if _, given := values[lf.name]; !given {
					promptable = append(promptable, in)
				}
			}
			// Non-@input leaves (e.g. a `name` filled from the key elsewhere) are
			// left alone; the final concreteness check covers `states`.
		}

		if len(promptable) > 0 {
			next := leastInput(promptable)
			pathOf[next.Name] = next.Path
			val, err := prompt(next)
			if err != nil {
				return err
			}
			values[next.Name] = val
			continue
		}

		// Scalars done — recurse into any nested user-keyed maps, then finish.
		sort.Slice(nested, func(i, j int) bool {
			if nested[i].Order != nested[j].Order {
				return nested[i].Order < nested[j].Order
			}
			return nested[i].Name < nested[j].Name
		})
		for _, dm := range nested {
			if err := resolveDynamicMap(ctx, base, dm, pathOf, values, prompt); err != nil {
				return err
			}
		}
		return nil
	}
}

// leastInput returns the input that sorts first by (order, name).
func leastInput(in []Input) Input {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Order != in[j].Order {
			return in[i].Order < in[j].Order
		}
		return in[i].Name < in[j].Name
	})
	return in[0]
}

// leafLabel is the last path segment of a dotted name, for display.
func leafLabel(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return name
}

// isYes reports whether a free-text answer is affirmative.
func isYes(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes", "true", "1":
		return true
	}
	return false
}

// finalValue rebuilds the chart with the resolved values injected as CUE source,
// so the result is identical to having written those values in the chart (which
// makes struct comprehensions with computed labels concrete).
func finalValue(ctx *cue.Context, chartDir string, opts Opts, pkg string, values map[string]string, pathOf map[string]cue.Path, dynMaps []DynMap) (cue.Value, error) {
	if len(values) == 0 && len(dynMaps) == 0 {
		_, v, _, err := buildBase(chartDir, opts, nil)
		return v, err
	}
	src, err := valuesToCUE(ctx, pkg, values, pathOf, dynMaps)
	if err != nil {
		return cue.Value{}, err
	}
	_, v, _, err := buildBase(chartDir, opts, map[string]string{"coffeeenv_values.cue": src})
	return v, err
}

// valuesToCUE renders the resolved values as a CUE source file in the chart's
// package, e.g. `package env\nprojects: {myrepo: {repoPath: "/r", ...}}`. Each
// @inputMap field is emitted as a closed struct of exactly its resolved keys —
// even when empty — so iterating it downstream yields a concrete (not open,
// "incomplete") set.
func valuesToCUE(ctx *cue.Context, pkg string, values map[string]string, pathOf map[string]cue.Path, dynMaps []DynMap) (string, error) {
	fills := ctx.CompileString("{}")
	// Seed each @inputMap field with `{}` so one that ended up with no keys still
	// renders as `field: {}` (concrete) rather than being absent (and left as the
	// chart's open pattern).
	for _, dm := range dynMaps {
		fills = fills.FillPath(dm.Path, ctx.CompileString("{}"))
	}
	for k, v := range values {
		p, ok := pathOf[k]
		if !ok {
			p = cue.ParsePath(k)
			if err := p.Err(); err != nil {
				return "", fmt.Errorf("invalid value path %q: %w", k, err)
			}
		}
		fills = fills.FillPath(p, encodeTyped(ctx, v))
	}
	if err := fills.Err(); err != nil {
		return "", err
	}
	node := fills.Syntax(cue.Concrete(true), cue.Final())
	st, ok := node.(*ast.StructLit)
	if !ok {
		return "", fmt.Errorf("unexpected values syntax %T", node)
	}
	file := &ast.File{Decls: []ast.Decl{&ast.Package{Name: ast.NewIdent(pkg)}}}
	file.Decls = append(file.Decls, st.Elts...)
	b, err := format.Node(file)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// scanInputs walks the chart value (skipping the `states` output and
// hidden/definition fields) and collects every scalar leaf, the subset
// annotated with @input, and every @inputMap (user-keyed map) field.
func scanInputs(base cue.Value) (map[string]Input, []leaf, []DynMap, error) {
	inputs := map[string]Input{}
	var leaves []leaf
	var dynMaps []DynMap

	it, err := base.Fields()
	if err != nil {
		return nil, nil, nil, err
	}
	for it.Next() {
		if it.Selector().String() == "states" {
			continue
		}
		scanValue(it.Value(), []cue.Selector{it.Selector()}, inputs, &leaves, &dynMaps)
	}
	return inputs, leaves, dynMaps, nil
}

// scanValue recurses structs and concrete-length lists; scalar leaves are
// recorded, and those carrying @input become promptable inputs. A struct field
// carrying @inputMap is recorded as a DynMap and not recursed (its keys are
// user-provided), so its pattern-constrained value schema is resolved per key.
func scanValue(v cue.Value, sels []cue.Selector, inputs map[string]Input, leaves *[]leaf, dynMaps *[]DynMap) {
	// A field annotated @inputMap is a user-keyed map: record it and don't recurse
	// (keys come from prompts/--value, resolved per entry against the pattern).
	if attr := v.Attribute("inputMap"); attr.Err() == nil && dynMaps != nil {
		path := cue.MakePath(sels...)
		prompt, _ := attr.String(0)
		*dynMaps = append(*dynMaps, DynMap{Name: path.String(), Path: path, Prompt: prompt, Order: attrOrder(attr)})
		return
	}

	if v.IncompleteKind() == cue.ListKind {
		n, err := v.Len().Int64()
		if err != nil {
			return // open/unknown-length list — nothing to prompt
		}
		for i := int64(0); i < n; i++ {
			elem := v.LookupPath(cue.MakePath(cue.Index(int(i))))
			scanValue(elem, append(clone(sels), cue.Index(int(i))), inputs, leaves, dynMaps)
		}
		return
	}

	// Struct detection via Fields() rather than IncompleteKind: a struct whose
	// child carries an unresolved disjunction (e.g. authType: "a" | "b") reads as
	// bottom for IncompleteKind, but still enumerates its fields here.
	if it, err := v.Fields(); err == nil {
		any := false
		for it.Next() {
			any = true
			scanValue(it.Value(), append(clone(sels), it.Selector()), inputs, leaves, dynMaps)
		}
		if any || v.IncompleteKind() == cue.StructKind {
			return
		}
	}

	// Scalar leaf (string/int/bool/number/disjunction-of-scalars/...).
	path := cue.MakePath(sels...)
	name := path.String()
	*leaves = append(*leaves, leaf{name: name, path: path})
	attr := v.Attribute("input")
	if attr.Err() != nil {
		return
	}
	prompt, _ := attr.String(0)
	inputs[name] = Input{Name: name, Path: path, Prompt: prompt, Order: attrOrder(attr)}
}

// attrOrder reads the order=N key from an @input/@inputMap attribute, defaulting
// to "last" when absent or unparseable.
func attrOrder(attr cue.Attribute) int {
	order := math.MaxInt32
	if s, found, _ := attr.Lookup(0, "order"); found {
		if k, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			order = k
		}
	}
	return order
}

func clone(s []cue.Selector) []cue.Selector {
	return append([]cue.Selector{}, s...)
}

// fillValues builds a standalone "fills" value from the given key=val pairs and
// unifies it into base. We use Unify (not FillPath into base directly) because
// FillPath does not re-trigger struct comprehensions with computed labels (e.g.
// `{for p in projects {(p.name): …}}`); standard unification does. The fills
// value itself has no comprehensions, so FillPath is safe to construct it.
func fillValues(ctx *cue.Context, base cue.Value, values map[string]string, pathOf map[string]cue.Path) (cue.Value, error) {
	if len(values) == 0 {
		return base, nil
	}
	fills := ctx.CompileString("{}")
	for k, v := range values {
		p, ok := pathOf[k]
		if !ok {
			p = cue.ParsePath(k)
			if err := p.Err(); err != nil {
				return cue.Value{}, fmt.Errorf("invalid value path %q: %w", k, err)
			}
		}
		fills = fills.FillPath(p, encodeTyped(ctx, v))
	}
	cur := base.Unify(fills)
	if err := cur.Err(); err != nil {
		return cue.Value{}, err
	}
	return cur, nil
}

// leafFixed reports whether a scalar field is concrete in cur.
func leafFixed(cur cue.Value, path cue.Path) bool {
	fv := cur.LookupPath(path)
	return fv.Exists() && fv.Validate(cue.Concrete(true)) == nil
}

// encodeTyped turns a flag string into a typed cue.Value: true/false -> bool,
// integer/float -> number, otherwise string.
func encodeTyped(ctx *cue.Context, s string) cue.Value {
	if s == "true" || s == "false" {
		b, _ := strconv.ParseBool(s)
		return ctx.Encode(b)
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return ctx.Encode(i)
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return ctx.Encode(f)
	}
	return ctx.Encode(s)
}

func missingInputsErr(promptable []Input, plain []string) error {
	var b strings.Builder
	b.WriteString("missing inputs (pass --value PATH=...):")
	sort.Slice(promptable, func(i, j int) bool { return promptable[i].Name < promptable[j].Name })
	for _, in := range promptable {
		fmt.Fprintf(&b, "\n  %s — %s", in.Name, in.Prompt)
	}
	for _, n := range plain {
		fmt.Fprintf(&b, "\n  %s (no @input annotation)", n)
	}
	return fmt.Errorf("%s", b.String())
}

func unresolvedErr(cur cue.Value, pathOf map[string]cue.Path, badGiven, plain []string) error {
	var b strings.Builder
	b.WriteString("unresolved fields:")
	for _, n := range badGiven {
		reason := ""
		if err := cur.LookupPath(pathOf[n]).Err(); err != nil {
			reason = ": " + err.Error()
		}
		fmt.Fprintf(&b, "\n  %s (value conflicts%s)", n, reason)
	}
	for _, n := range plain {
		fmt.Fprintf(&b, "\n  %s (not concrete and has no @input annotation)", n)
	}
	return fmt.Errorf("%s", b.String())
}
