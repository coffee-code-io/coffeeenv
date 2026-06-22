// Package cmd implements the coffeeenv cobra CLI: pull, plan, apply, venv.
package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/coffee-code-io/coffeeenv/internal/chart"
	"github.com/coffee-code-io/coffeeenv/internal/cuelib"
	"github.com/coffee-code-io/coffeeenv/internal/state"
	"github.com/coffee-code-io/coffeeenv/internal/venv"
)

var rootCmd = &cobra.Command{
	Use:   "coffeeenv",
	Short: "Declarative environment manager for AI coding setups",
	Long: `coffeeenv renders a CUE chart into states and converges them.

Workflow:
  coffeeenv pull <source>          fetch a CUE chart into ~/.coffeeenv/charts
  coffeeenv plan  <chart>          show what would change on this machine
  coffeeenv apply <chart>          converge this machine to the chart
  coffeeenv venv create <name>     make a local environment
  coffeeenv apply --venv <name> <chart>   install the chart into the venv
  coffeeenv apply --materialize <name>    re-render the venv's chart globally`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(pullCmd, planCmd, applyCmd, planSkillCmd, applySkillCmd, venvCmd)
}

func globalRoot() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return "~"
}

// target is a resolved plan/apply destination: the composition manifest (its
// accumulated execs + values), the engine context, and how to persist the
// updated manifest after apply (nil for --materialize, which doesn't mutate).
type target struct {
	manifest chart.Manifest
	opts     cuelib.Opts
	save     func(chart.Manifest) error
	local    bool   // venv (local engine)
	venvName string // set in --venv mode (for the activate hint)
	label    string
}

// loadTarget opens the composition manifest for the chosen scope (global, or a
// named venv) along with its engine opts, save func, and label.
func loadTarget(venvName string) (target, error) {
	if venvName != "" {
		v, err := venv.Open(venvName)
		if err != nil {
			return target{}, err
		}
		if !v.Exists() {
			return target{}, fmt.Errorf("no venv %q — run `coffeeenv venv create %s` first", venvName, venvName)
		}
		m, err := v.ReadManifest()
		if err != nil {
			return target{}, fmt.Errorf("read venv manifest: %w", err)
		}
		return target{
			manifest: m,
			opts:     cuelib.Opts{Engine: "local", Root: v.Dir},
			save:     v.WriteManifest,
			local:    true,
			venvName: venvName,
			label:    fmt.Sprintf("venv %s", venvName),
		}, nil
	}
	m, err := readGlobalManifest()
	if err != nil {
		return target{}, err
	}
	return target{
		manifest: m,
		opts:     cuelib.Opts{Engine: "global", Root: globalRoot()},
		save:     writeGlobalManifest,
		label:    "global",
	}, nil
}

// resolveTarget turns the plan/apply flags into a composition target. The three
// modes (default global, --venv local, --materialize global-from-manifest) are
// mutually exclusive. Applying a chart accumulates it into the manifest's execs;
// a chart arg that is a git/oci/local source is pulled first (deduped).
func resolveTarget(ctx context.Context, chartArg, venvName, materialize string, values map[string]string) (target, error) {
	if materialize != "" {
		if venvName != "" || chartArg != "" {
			return target{}, fmt.Errorf("--materialize cannot be combined with --venv or a chart argument")
		}
		v, err := venv.Open(materialize)
		if err != nil {
			return target{}, err
		}
		if !v.Exists() {
			return target{}, fmt.Errorf("no venv %q — run `coffeeenv venv create %s` first", materialize, materialize)
		}
		m, err := v.ReadManifest()
		if err != nil {
			return target{}, fmt.Errorf("read venv manifest: %w", err)
		}
		if len(m.Execs) == 0 {
			return target{}, fmt.Errorf("venv %q has no charts installed; run `coffeeenv apply --venv %s <chart>` first", materialize, materialize)
		}
		m.Values = mergeValues(m.Values, values)
		return target{
			manifest: m,
			opts:     cuelib.Opts{Engine: "global", Root: globalRoot()},
			label:    fmt.Sprintf("materialize %s", materialize),
		}, nil
	}

	t, err := loadTarget(venvName)
	if err != nil {
		return target{}, err
	}
	if chartArg != "" && chart.IsSource(chartArg) {
		name, err := pullChart(ctx, chartArg)
		if err != nil {
			return target{}, err
		}
		chartArg = name
	}
	if t.manifest, err = accumulate(t.manifest, chartArg, values); err != nil {
		return target{}, err
	}
	return t, nil
}

// resolveSkillTarget is resolveTarget for a skill: it pulls the skill source
// (deduped) and accumulates it into the manifest's skills (not execs). Skills
// attach to the global setup or a venv; --materialize is not supported.
func resolveSkillTarget(ctx context.Context, skillArg, venvName string, values map[string]string) (target, error) {
	t, err := loadTarget(venvName)
	if err != nil {
		return target{}, err
	}
	name, err := pullSkill(ctx, skillArg)
	if err != nil {
		return target{}, err
	}
	t.manifest = accumulateSkill(t.manifest, name, values)
	return t, nil
}

// accumulate adds chartArg's module to the manifest's execs (dedup) and merges
// its values, then layers the --value flags on top.
func accumulate(m chart.Manifest, chartArg string, flags map[string]string) (chart.Manifest, error) {
	if chartArg != "" {
		c, err := resolveChart(chartArg)
		if err != nil {
			return m, err
		}
		cm, ok, err := c.ReadManifest()
		if err != nil {
			return m, err
		}
		if !ok || cm.Module == "" {
			return m, fmt.Errorf("chart %q has no manifest.json with a module (not an executable chart)", c.Name)
		}
		m.Execs = appendDedup(m.Execs, cm.Module)
		m.Values = mergeValues(m.Values, cm.Values)
	}
	m.Values = mergeValues(m.Values, flags)
	return m, nil
}

// accumulateSkill adds a pulled skill's name to the manifest's skills (dedup) and
// layers the --value flags on top.
func accumulateSkill(m chart.Manifest, skillName string, flags map[string]string) chart.Manifest {
	m.Skills = appendDedup(m.Skills, skillName)
	m.Values = mergeValues(m.Values, flags)
	return m
}

// parseValues turns repeated --value key=val flags into a map.
func parseValues(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --value %q (want key=val)", p)
		}
		m[k] = v
	}
	return m, nil
}

// resolveChart resolves a chart by name, defaulting to the sole chart when the
// name is empty and exactly one chart exists.
func resolveChart(name string) (chart.Chart, error) {
	if name == "" {
		names, err := chart.List()
		if err != nil {
			return chart.Chart{}, err
		}
		switch len(names) {
		case 0:
			return chart.Chart{}, fmt.Errorf("no charts pulled — run `coffeeenv pull <source>` first")
		case 1:
			name = names[0]
		default:
			return chart.Chart{}, fmt.Errorf("multiple charts exist; specify one of: %v", names)
		}
	}
	c, err := chart.Open(name)
	if err != nil {
		return chart.Chart{}, err
	}
	if !c.Exists() {
		return chart.Chart{}, fmt.Errorf("no chart %q in ~/.coffeeenv/charts", name)
	}
	return c, nil
}

// computePlan composes the target's execs into states, decodes them, and diffs
// against the system. It returns the plan and the resolved values map (to persist
// back into the manifest). prompt is nil for non-interactive callers.
func computePlan(ctx context.Context, t target, prompt cuelib.PromptFunc) (state.Plan, map[string]string, error) {
	deps, err := depsIndex()
	if err != nil {
		return state.Plan{}, nil, err
	}
	skillDirs, err := skillDirsFor(t.manifest.Skills)
	if err != nil {
		return state.Plan{}, nil, err
	}
	r, err := cuelib.Compose(t.manifest.Execs, skillDirs, deps, t.opts, t.manifest.Values, prompt)
	if err != nil {
		return state.Plan{}, nil, err
	}
	resolved, err := state.DecodeStates(r.States)
	if err != nil {
		return state.Plan{}, nil, err
	}
	p, err := state.Engine{}.Plan(ctx, resolved)
	return p, r.Values, err
}
