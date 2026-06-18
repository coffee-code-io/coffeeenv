package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/coffee-code-io/coffeeenv/internal/state"
)

// Plan-preview palette. color auto-disables when stdout is not a TTY or NO_COLOR
// is set, so the output is plain when piped.
var (
	cInstall = color.New(color.FgGreen)
	cWrite   = color.New(color.FgYellow)
	cRun     = color.New(color.FgCyan)
	cName    = color.New(color.Bold)
	cDim     = color.New(color.Faint)
	cSummary = color.New(color.Bold)
)

var (
	planJSON        bool
	planVenv        string
	planMaterialize string
	planValues      []string
)

var planCmd = &cobra.Command{
	Use:   "plan [chart]",
	Short: "Show what would change",
	Long: `Render a chart and show the actions needed to converge.

[chart] is a pulled chart name or a git/oci/local source (pulled first, deduped).

Modes:
  plan [chart]              against the real system (engine=global)
  plan --venv <name> <chart>   into a local venv (engine=local)
  plan --materialize <name>    re-render the venv's chart against the real system`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		values, err := parseValues(planValues)
		if err != nil {
			return err
		}
		t, err := resolveTarget(cmd.Context(), firstArg(args), planVenv, planMaterialize, values)
		if err != nil {
			return err
		}
		// plan never prompts: a nil PromptFunc errors listing missing inputs.
		p, _, err := computePlan(cmd.Context(), t, nil)
		if err != nil {
			return err
		}
		if planJSON {
			return printPlanJSON(p)
		}
		fmt.Printf("Target: %s\n", t.label)
		printPlan(p)
		return nil
	},
}

func init() {
	planCmd.Flags().BoolVar(&planJSON, "json", false, "emit the action list as JSON")
	planCmd.Flags().StringVar(&planVenv, "venv", "", "render into the named venv (engine=local)")
	planCmd.Flags().StringVar(&planMaterialize, "materialize", "", "re-render the named venv's chart globally")
	planCmd.Flags().StringArrayVarP(&planValues, "value", "V", nil, "set an input value: key=val (repeatable)")
}

func firstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// printPlan renders a terraform-style summary of the plan, colored on a TTY.
func printPlan(p state.Plan) {
	for _, a := range p.Actions {
		sigil, c := marker(a.Kind)
		fmt.Printf("  %s %s %s\n",
			c.Sprint(sigil),
			cName.Sprintf("%-12s", a.StateName),
			cDim.Sprint(a.Summary))
	}
	if len(p.Actions) == 0 {
		fmt.Printf("No changes. %d state(s) already up to date.\n", p.Unchanged)
		return
	}
	fmt.Printf("\n%s\n", cSummary.Sprintf("Plan: %d to change, %d unchanged.", len(p.Actions), p.Unchanged))
}

// marker maps an action kind to a leading sigil and its color for plan output.
func marker(kind string) (string, *color.Color) {
	switch kind {
	case "write-file", "set-env":
		return "~", cWrite
	case "run":
		return ">", cRun
	default: // installs / copies
		return "+", cInstall
	}
}

func printPlanJSON(p state.Plan) error {
	type actJSON struct {
		State   string `json:"state"`
		Kind    string `json:"kind"`
		Summary string `json:"summary"`
	}
	out := struct {
		Actions   []actJSON `json:"actions"`
		Unchanged int       `json:"unchanged"`
	}{Unchanged: p.Unchanged}
	for _, a := range p.Actions {
		out.Actions = append(out.Actions, actJSON{State: a.StateName, Kind: a.Kind, Summary: a.Summary})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
