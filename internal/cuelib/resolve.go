package cuelib

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"

	"github.com/coffee-code-io/coffeeenv/internal/state"
)

// InputKind is how an input is collected from the user.
type InputKind int

const (
	KindText        InputKind = iota // free text
	KindChoose                       // single-select from a fixed option set
	KindMultichoice                  // multi-select from a fixed option set -> a list
	KindConfirm                      // yes/no
)

// Input is a chart field the resolver needs a value for. Name is the field's CUE
// path (also the key in the values map); Path is the selector path for filling.
// Kind selects the prompt widget; Options carries a fixed option set (for
// choose/multichoice, and as an inline hint for small disjunction text inputs).
type Input struct {
	Name    string
	Path    cue.Path
	Prompt  string
	Order   int
	Kind    InputKind
	Options []string
}

// PromptFunc resolves an input value interactively. For KindMultichoice it
// returns the selected options joined by commas. A nil PromptFunc means
// non-interactive: unresolved inputs become an error.
type PromptFunc func(Input) (string, error)

// Result is the outcome of resolution: the final values map (for the manifest)
// and the decoded flat states list.
type Result struct {
	Values map[string]string
	States []state.RawState
}

// leaf is a scalar/list field discovered during the scan.
type leaf struct {
	name string
	path cue.Path
}

// inputSpec is an annotated promptable field discovered during the scan. `from`
// is the path of a registry whose keys are the option set (resolved at process
// time); `static` is a fixed option set extracted from a disjunction type.
type inputSpec struct {
	name   string
	path   cue.Path
	prompt string
	order  int
	kind   InputKind
	from   string
	static []string
}

func (s inputSpec) toInput(opts []string) Input {
	return Input{Name: s.name, Path: s.path, Prompt: s.prompt, Order: s.order, Kind: s.kind, Options: opts}
}

// DynMap is a keyed-map field annotated @inputMap("<key prompt>", order=N) whose
// entries are supplied interactively: the resolver prompts for a key, resolves
// that entry's own inputs, then asks whether to add another.
//
// A map that ends up empty is injected as `{}` so downstream stays concrete.
type DynMap struct {
	Name   string
	Path   cue.Path
	Prompt string
	Order  int
}

// step is one unit of the unified ordered resolution pass: either a scalar
// input or a user-keyed map. Steps are processed in (order, name) order so an
// input can depend on an earlier one (e.g. the active project chooses among the
// projects entered by an earlier @inputMap).
type step struct {
	order int
	name  string
	input *inputSpec // non-nil for a scalar step
	dyn   *DynMap    // non-nil for a map step
}

// Resolve builds the chart, fills in the given values, resolves the remaining
// inputs in a single order-respecting pass (re-rendering after each so CUE
// propagation can pin dependents), then extracts the states.
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

	// Unified ordered pass over scalar inputs and @inputMaps.
	steps := buildSteps(inputs, dynMaps)
	var missing []Input
	for _, s := range steps {
		cur, err := fillValues(ctx, base, values, pathOf)
		if err != nil {
			return Result{}, err
		}
		if s.dyn != nil {
			if err := resolveDynamicMap(ctx, base, *s.dyn, pathOf, values, prompt); err != nil {
				return Result{}, err
			}
			continue
		}
		spec := s.input
		if _, isGiven := values[spec.name]; isGiven {
			continue
		}
		if inputResolved(cur, *spec) {
			continue
		}
		opts := buildOptions(cur, *spec)
		if (spec.kind == KindChoose || spec.kind == KindMultichoice) && len(opts) == 0 {
			// No options yet (e.g. active project before any project exists):
			// resolve to empty so the field stays concrete and is simply unset.
			values[spec.name] = ""
			continue
		}
		if prompt == nil {
			missing = append(missing, spec.toInput(opts))
			continue
		}
		val, err := prompt(spec.toInput(opts))
		if err != nil {
			return Result{}, err
		}
		values[spec.name] = val
	}

	// Final validation over the statically-discovered leaves.
	if err := validateLeaves(ctx, base, inputs, leaves, pathOf, values, missing); err != nil {
		return Result{}, err
	}

	// Rebuild with the resolved values injected as source so comprehensions with
	// computed labels re-evaluate, then extract `states`.
	final, err := finalValue(ctx, base, chartDir, opts, pkg, values, pathOf, dynMaps)
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
	resolveCopySrc(raws, statesV, chartDir, opts.Deps)
	return Result{Values: values, States: raws}, nil
}

// buildSteps merges scalar inputs and @inputMaps into one list sorted by
// (order, name) for a single dependency-respecting resolution pass.
func buildSteps(inputs map[string]inputSpec, dynMaps []DynMap) []step {
	steps := make([]step, 0, len(inputs)+len(dynMaps))
	for name := range inputs {
		s := inputs[name]
		steps = append(steps, step{order: s.order, name: s.name, input: &s})
	}
	for i := range dynMaps {
		dm := dynMaps[i]
		steps = append(steps, step{order: dm.Order, name: dm.Name, dyn: &dm})
	}
	sort.Slice(steps, func(i, j int) bool {
		if steps[i].order != steps[j].order {
			return steps[i].order < steps[j].order
		}
		return steps[i].name < steps[j].name
	})
	return steps
}

// buildOptions computes the option set for a choose/multichoice input: the keys
// of the `from` registry (resolved against cur) or the static disjunction set.
func buildOptions(cur cue.Value, spec inputSpec) []string {
	if spec.from != "" {
		return keysOrElems(cur.LookupPath(cue.ParsePath(spec.from)))
	}
	return spec.static
}

// keysOrElems returns a struct's field names or a list's string elements.
func keysOrElems(v cue.Value) []string {
	if !v.Exists() {
		return nil
	}
	if it, err := v.List(); err == nil {
		var out []string
		for it.Next() {
			if s, err := it.Value().String(); err == nil {
				out = append(out, s)
			}
		}
		return out
	}
	if it, err := v.Fields(); err == nil {
		var out []string
		for it.Next() {
			sel := it.Selector()
			// Unquoted() for string labels: a key like "my-repo" must be offered
			// (and stored) as my-repo, not the quoted "my-repo" that String()
			// returns — otherwise the quotes leak into the chosen value.
			if sel.IsString() {
				out = append(out, sel.Unquoted())
			} else {
				out = append(out, sel.String())
			}
		}
		return out
	}
	return nil
}

// validateLeaves reports the final error: missing promptable inputs (when
// non-interactive), leaves with no @input annotation, or given values that
// conflict. It only covers the statically-scanned top-level leaves; per-entry
// concreteness is enforced by the states validation.
func validateLeaves(ctx *cue.Context, base cue.Value, inputs map[string]inputSpec, leaves []leaf, pathOf map[string]cue.Path, values map[string]string, missing []Input) error {
	cur, err := fillValues(ctx, base, values, pathOf)
	if err != nil {
		return err
	}
	var plain, badGiven []string
	for _, lf := range leaves {
		if leafFixed(cur, lf.path) {
			continue
		}
		if _, isInput := inputs[lf.name]; isInput {
			if _, given := values[lf.name]; given {
				badGiven = append(badGiven, lf.name)
			}
		} else {
			plain = append(plain, lf.name)
		}
	}
	if len(missing) > 0 || len(plain) > 0 {
		return missingInputsErr(missing, plain)
	}
	if len(badGiven) > 0 {
		return unresolvedErr(cur, pathOf, badGiven, nil)
	}
	return nil
}

// resolveCopySrc rewrites each copy state's relative `src` to an absolute path
// anchored at the chart directory that supplied the src value. For dependency
// charts, CUE source positions point at the mounted cue.mod/pkg/<module>/...
// overlay; map that back to the pulled dependency directory so bundled files
// are copied from the dependency, not the importing chart.
func resolveCopySrc(raws []state.RawState, statesV cue.Value, chartDir string, deps map[string]string) {
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
		srcBase := copySrcBase(raws[i], i, statesV, base, deps)
		if _, err := os.Stat(filepath.Join(srcBase, src)); err != nil {
			if depBase, ok := depBaseWithCopySrc(src, deps); ok {
				srcBase = depBase
			}
		}
		raws[i].Params["src"] = filepath.Join(srcBase, src)
	}
}

func copySrcBase(rs state.RawState, index int, statesV cue.Value, chartBase string, deps map[string]string) string {
	path := cue.MakePath(cue.Index(index), cue.Str("src"))
	if rs.Name != "" {
		path = cue.MakePath(cue.Str(rs.Name), cue.Str("src"))
	}
	v := statesV.LookupPath(path)
	if !v.Exists() {
		return chartBase
	}
	filename := v.Pos().Filename()
	if filename == "" {
		return chartBase
	}
	for module, dir := range deps {
		depBase, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		overlayBase := filepath.Join(chartBase, "cue.mod", "pkg", filepath.FromSlash(module))
		if filename == overlayBase || strings.HasPrefix(filename, overlayBase+string(os.PathSeparator)) {
			return depBase
		}
	}
	return chartBase
}

func depBaseWithCopySrc(src string, deps map[string]string) (string, bool) {
	var match string
	for _, dir := range deps {
		depBase, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(depBase, src)); err != nil {
			continue
		}
		if match != "" {
			return "", false
		}
		match = depBase
	}
	return match, match != ""
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
		key, err := prompt(Input{Name: dm.Name, Prompt: fmt.Sprintf("%s (leave empty to finish):", dm.Prompt), Kind: KindText})
		if err != nil {
			return err
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil
		}
		entry := cue.MakePath(append(dm.Path.Selectors(), cue.Str(key))...)
		if err := resolveEntry(ctx, base, entry, pathOf, values, prompt); err != nil {
			return err
		}
		ans, err := prompt(Input{Name: dm.Name, Prompt: fmt.Sprintf("Add another %s?", label), Kind: KindConfirm})
		if err != nil {
			return err
		}
		if !isYes(ans) {
			return nil
		}
	}
}

// resolveEntry resolves every input (and any nested @inputMap) under a single
// map entry at `entry`, by (order, name), re-evaluating after each so entry
// fields can pin one another.
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

		inputs := map[string]inputSpec{}
		var leaves []leaf
		var nested []DynMap
		scanValue(elem, entry.Selectors(), inputs, &leaves, &nested)

		var cand []inputSpec
		for _, lf := range leaves {
			spec, ok := inputs[lf.name]
			if !ok {
				continue // non-input leaf — left to the states validation
			}
			if _, given := values[lf.name]; given {
				continue
			}
			if inputResolved(probe, spec) {
				continue
			}
			cand = append(cand, spec)
		}

		if len(cand) > 0 {
			next := leastSpec(cand)
			pathOf[next.name] = next.path
			opts := buildOptions(probe, next)
			if (next.kind == KindChoose || next.kind == KindMultichoice) && len(opts) == 0 {
				values[next.name] = ""
				continue
			}
			val, err := prompt(next.toInput(opts))
			if err != nil {
				return err
			}
			values[next.name] = val
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

// leastSpec returns the spec that sorts first by (order, name).
func leastSpec(in []inputSpec) inputSpec {
	sort.Slice(in, func(i, j int) bool {
		if in[i].order != in[j].order {
			return in[i].order < in[j].order
		}
		return in[i].name < in[j].name
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
// so the result is identical to having written those values in the chart.
func finalValue(ctx *cue.Context, base cue.Value, chartDir string, opts Opts, pkg string, values map[string]string, pathOf map[string]cue.Path, dynMaps []DynMap) (cue.Value, error) {
	if len(values) == 0 && len(dynMaps) == 0 {
		_, v, _, err := buildBase(chartDir, opts, nil)
		return v, err
	}
	src, err := valuesToCUE(ctx, base, pkg, values, pathOf, dynMaps)
	if err != nil {
		return cue.Value{}, err
	}
	_, v, _, err := buildBase(chartDir, opts, map[string]string{"coffeeenv_values.cue": src})
	return v, err
}

// valuesToCUE renders the resolved values as a CUE source file in the chart's
// package. Each @inputMap field is seeded with `{}` so an empty one stays
// concrete; list-typed fields receive a CUE list (comma-split).
func valuesToCUE(ctx *cue.Context, base cue.Value, pkg string, values map[string]string, pathOf map[string]cue.Path, dynMaps []DynMap) (string, error) {
	fills := ctx.CompileString("{}")
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
		fills = fills.FillPath(p, encodeFieldValue(ctx, base, p, v))
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

// scanInputs walks the chart value (skipping the `states` output) and collects
// every scalar/list leaf, the annotated promptable inputs, and every @inputMap.
func scanInputs(base cue.Value) (map[string]inputSpec, []leaf, []DynMap, error) {
	inputs := map[string]inputSpec{}
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

// scanValue records inputs/maps/leaves. @inputMap, @choose, and @multichoice are
// checked before list/struct recursion (a @multichoice field is a list, so it
// must not be recursed into).
func scanValue(v cue.Value, sels []cue.Selector, inputs map[string]inputSpec, leaves *[]leaf, dynMaps *[]DynMap) {
	path := cue.MakePath(sels...)
	name := path.String()

	if attr := v.Attribute("inputMap"); attr.Err() == nil && dynMaps != nil {
		prompt, _ := attr.String(0)
		*dynMaps = append(*dynMaps, DynMap{Name: name, Path: path, Prompt: prompt, Order: attrOrder(attr)})
		return
	}
	if attr := v.Attribute("choose"); attr.Err() == nil {
		prompt, _ := attr.String(0)
		*leaves = append(*leaves, leaf{name: name, path: path})
		inputs[name] = inputSpec{
			name: name, path: path, prompt: prompt, order: attrOrder(attr),
			kind: KindChoose, from: attrStr(attr, "from"), static: disjunctOptions(v),
		}
		return
	}
	if attr := v.Attribute("multichoice"); attr.Err() == nil {
		prompt, _ := attr.String(0)
		*leaves = append(*leaves, leaf{name: name, path: path})
		inputs[name] = inputSpec{
			name: name, path: path, prompt: prompt, order: attrOrder(attr),
			kind: KindMultichoice, from: attrStr(attr, "from"), static: disjunctOptions(v),
		}
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

	// Struct detection via Fields(): a struct whose child carries an unresolved
	// disjunction reads as bottom for IncompleteKind but still enumerates here.
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

	// Scalar leaf.
	*leaves = append(*leaves, leaf{name: name, path: path})
	attr := v.Attribute("input")
	if attr.Err() != nil {
		return
	}
	prompt, _ := attr.String(0)
	inputs[name] = inputSpec{
		name: name, path: path, prompt: prompt, order: attrOrder(attr),
		kind: KindText, static: disjunctOptions(v),
	}
}

// disjunctOptions returns the concrete string disjuncts of a value (e.g.
// `"a"|"b"`), or nil if it is not such a disjunction or has too many options.
func disjunctOptions(v cue.Value) []string {
	op, args := v.Expr()
	if op != cue.OrOp {
		return nil
	}
	opts := make([]string, 0, len(args))
	for _, a := range args {
		s, err := a.String()
		if err != nil {
			return nil
		}
		opts = append(opts, s)
	}
	if len(opts) == 0 || len(opts) >= 10 {
		return nil
	}
	return opts
}

// attrOrder reads order=N from an attribute, defaulting to "last".
func attrOrder(attr cue.Attribute) int {
	order := math.MaxInt32
	if s, found, _ := attr.Lookup(0, "order"); found {
		if k, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			order = k
		}
	}
	return order
}

// attrStr reads a named string key from an attribute (e.g. from=<path>).
func attrStr(attr cue.Attribute, key string) string {
	if s, found, _ := attr.Lookup(0, key); found {
		return strings.TrimSpace(s)
	}
	return ""
}

func clone(s []cue.Selector) []cue.Selector {
	return append([]cue.Selector{}, s...)
}

// fillValues unifies the given key=val pairs into base. Unify (not direct
// FillPath into base) is used so struct comprehensions with computed labels
// re-trigger.
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
		fills = fills.FillPath(p, encodeFieldValue(ctx, base, p, v))
	}
	cur := base.Unify(fills)
	if err := cur.Err(); err != nil {
		return cue.Value{}, err
	}
	return cur, nil
}

// encodeFieldValue encodes a flag string for injection at path p. When the
// target field is a list (e.g. a @multichoice `[...string]`), the value is
// comma-split into a CUE list (empty -> empty list); otherwise it is a scalar
// (encodeTyped).
func encodeFieldValue(ctx *cue.Context, base cue.Value, p cue.Path, v string) cue.Value {
	if targetIsList(ctx, base, p) {
		parts := splitComma(v)
		vals := make([]cue.Value, len(parts))
		for i, s := range parts {
			vals[i] = ctx.Encode(s)
		}
		return ctx.NewList(vals...)
	}
	return encodeTyped(ctx, v)
}

// targetIsList reports whether the field at p is list-typed, materializing the
// map-key ancestors so pattern constraints (e.g. {[string]: #Project}) apply.
func targetIsList(ctx *cue.Context, base cue.Value, p cue.Path) bool {
	sels := p.Selectors()
	probe := base
	for i := 1; i < len(sels); i++ {
		sub := cue.MakePath(sels[:i]...)
		if !probe.LookupPath(sub).Exists() {
			probe = probe.FillPath(sub, ctx.CompileString("{}"))
		}
	}
	return probe.LookupPath(p).IncompleteKind() == cue.ListKind
}

// splitComma splits a comma list, trimming spaces and dropping empties.
func splitComma(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// leafFixed reports whether a field is concrete in cur.
func leafFixed(cur cue.Value, path cue.Path) bool {
	fv := cur.LookupPath(path)
	return fv.Exists() && fv.Validate(cue.Concrete(true)) == nil
}

// inputResolved reports whether an input no longer needs prompting. For a
// multichoice (a list) the open schema `[...string]` validates as concrete
// (empty), so resolution is instead "has a concrete length" — true only once a
// real list (incl. an explicit empty one) has been injected.
func inputResolved(cur cue.Value, spec inputSpec) bool {
	fv := cur.LookupPath(spec.path)
	if !fv.Exists() {
		return false
	}
	if spec.kind == KindMultichoice {
		_, err := fv.Len().Int64()
		return err == nil
	}
	return fv.Validate(cue.Concrete(true)) == nil
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
