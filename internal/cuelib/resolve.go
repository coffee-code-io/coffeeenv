package cuelib

import (
	"fmt"
	"math"
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

// Resolve builds the chart, fills in the given values, and iteratively resolves
// remaining @input fields at any depth (re-rendering after each so CUE
// propagation can fix dependents). A non-annotated non-fixed scalar leaf is an
// error.
func Resolve(chartDir string, opts Opts, given map[string]string, prompt PromptFunc) (Result, error) {
	ctx, base, pkg, err := buildBase(chartDir, opts, nil)
	if err != nil {
		return Result{}, err
	}

	inputs, leaves, err := scanInputs(base)
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

	for {
		cur, err := fillValues(ctx, base, values, pathOf)
		if err != nil {
			return Result{}, err
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
				return Result{}, missingInputsErr(promptable, plain)
			}
			sort.Slice(promptable, func(i, j int) bool {
				if promptable[i].Order != promptable[j].Order {
					return promptable[i].Order < promptable[j].Order
				}
				return promptable[i].Name < promptable[j].Name
			})
			next := promptable[0]
			val, err := prompt(next)
			if err != nil {
				return Result{}, err
			}
			values[next.Name] = val
			continue
		}

		if len(badGiven) > 0 || len(plain) > 0 {
			return Result{}, unresolvedErr(cur, pathOf, badGiven, plain)
		}

		// All inputs resolved. Re-build the chart with the resolved values
		// injected as a SOURCE overlay (not FillPath/Unify) so struct
		// comprehensions with computed labels re-evaluate against concrete data.
		final, err := finalValue(ctx, chartDir, opts, pkg, values, pathOf)
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
		return Result{Values: values, States: raws}, nil
	}
}

// finalValue rebuilds the chart with the resolved values injected as CUE source,
// so the result is identical to having written those values in the chart (which
// makes struct comprehensions with computed labels concrete).
func finalValue(ctx *cue.Context, chartDir string, opts Opts, pkg string, values map[string]string, pathOf map[string]cue.Path) (cue.Value, error) {
	if len(values) == 0 {
		_, v, _, err := buildBase(chartDir, opts, nil)
		return v, err
	}
	src, err := valuesToCUE(ctx, pkg, values, pathOf)
	if err != nil {
		return cue.Value{}, err
	}
	_, v, _, err := buildBase(chartDir, opts, map[string]string{"coffeeenv_values.cue": src})
	return v, err
}

// valuesToCUE renders the resolved values as a CUE source file in the chart's
// package, e.g. `package env\nprojects: [{repoPath: "/r", ...}]`.
func valuesToCUE(ctx *cue.Context, pkg string, values map[string]string, pathOf map[string]cue.Path) (string, error) {
	fills := ctx.CompileString("{}")
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
// hidden/definition fields) and collects every scalar leaf plus the subset
// annotated with @input.
func scanInputs(base cue.Value) (map[string]Input, []leaf, error) {
	inputs := map[string]Input{}
	var leaves []leaf

	it, err := base.Fields()
	if err != nil {
		return nil, nil, err
	}
	for it.Next() {
		if it.Selector().String() == "states" {
			continue
		}
		scanValue(it.Value(), []cue.Selector{it.Selector()}, inputs, &leaves)
	}
	return inputs, leaves, nil
}

// scanValue recurses structs and concrete-length lists; scalar leaves are
// recorded, and those carrying @input become promptable inputs.
func scanValue(v cue.Value, sels []cue.Selector, inputs map[string]Input, leaves *[]leaf) {
	switch v.IncompleteKind() {
	case cue.StructKind:
		it, err := v.Fields()
		if err != nil {
			return
		}
		for it.Next() {
			scanValue(it.Value(), append(clone(sels), it.Selector()), inputs, leaves)
		}
	case cue.ListKind:
		n, err := v.Len().Int64()
		if err != nil {
			return // open/unknown-length list — nothing to prompt
		}
		for i := int64(0); i < n; i++ {
			elem := v.LookupPath(cue.MakePath(cue.Index(int(i))))
			scanValue(elem, append(clone(sels), cue.Index(int(i))), inputs, leaves)
		}
	default: // scalar leaf (string/int/bool/number/...)
		path := cue.MakePath(sels...)
		name := path.String()
		*leaves = append(*leaves, leaf{name: name, path: path})
		attr := v.Attribute("input")
		if attr.Err() != nil {
			return
		}
		prompt, _ := attr.String(0)
		order := math.MaxInt32
		if s, found, _ := attr.Lookup(0, "order"); found {
			if k, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				order = k
			}
		}
		inputs[name] = Input{Name: name, Path: path, Prompt: prompt, Order: order}
	}
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
