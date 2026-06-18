package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/coffee-code-io/coffeeenv/internal/cuelib"
	"github.com/coffee-code-io/coffeeenv/internal/state"
)

var (
	autoApprove      bool
	applyVenv        string
	applyMaterialize string
	applyValues      []string
)

var applyCmd = &cobra.Command{
	Use:   "apply [chart]",
	Short: "Converge the chart's states",
	Long: `Render a chart and apply the actions needed to converge.

[chart] is a pulled chart name or a git/oci/local source (pulled first, deduped).

Modes:
  apply [chart]              against the real system (engine=global)
  apply --venv <name> <chart>   install into a local venv (engine=local)
  apply --materialize <name>    re-render the venv's chart against the real system`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		values, err := parseValues(applyValues)
		if err != nil {
			return err
		}
		t, err := resolveTarget(cmd.Context(), firstArg(args), applyVenv, applyMaterialize, values)
		if err != nil {
			return err
		}
		// apply prompts for unresolved inputs only on a TTY; materialize never prompts.
		return executeApply(cmd, t, applyMaterialize == "")
	},
}

// executeApply computes the target's plan, prints it, confirms (unless
// --auto-approve), applies it, and persists the accumulated manifest. allowPrompt
// gates interactive input prompting (still requires a TTY).
func executeApply(cmd *cobra.Command, t target, allowPrompt bool) error {
	var prompt cuelib.PromptFunc
	if allowPrompt && stdinIsTTY() {
		prompt = interactivePrompt
	}

	p, resolvedValues, err := computePlan(cmd.Context(), t, prompt)
	if err != nil {
		return err
	}
	// Persist the accumulated composition (execs/skills + resolved values) back to
	// the target manifest, so the venv/global setup is the union of all applies.
	t.manifest.Values = resolvedValues
	saveManifest := func() error {
		if t.save == nil {
			return nil
		}
		return t.save(t.manifest)
	}

	fmt.Printf("Target: %s\n", t.label)
	if len(p.Actions) == 0 {
		fmt.Printf("Nothing to do. %d state(s) already up to date.\n", p.Unchanged)
		return saveManifest()
	}

	printPlan(p)
	if !autoApprove {
		ok, err := confirm("\nApply these changes?")
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("Aborted.")
			return nil
		}
	}

	fmt.Println()
	if err := (state.Engine{}).Apply(cmd.Context(), p); err != nil {
		return err
	}
	fmt.Printf("\nApplied %d change(s).\n", len(p.Actions))

	if err := saveManifest(); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}
	printEnvHintIfNeeded(t, p)
	return nil
}

func init() {
	applyCmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "apply without prompting")
	applyCmd.Flags().StringVar(&applyVenv, "venv", "", "install into the named venv (engine=local)")
	applyCmd.Flags().StringVar(&applyMaterialize, "materialize", "", "re-render the named venv's chart globally")
	applyCmd.Flags().StringArrayVarP(&applyValues, "value", "V", nil, "set an input value: key=val (repeatable)")
}

// stdinIsTTY reports whether stdin is an interactive terminal.
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// printEnvHintIfNeeded reminds the user how to load env vars that were set.
func printEnvHintIfNeeded(t target, p state.Plan) {
	for _, a := range p.Actions {
		if a.Kind != "set-env" {
			continue
		}
		if t.local {
			fmt.Printf("\nEnv vars written to the venv. Activate with:\n  coffeeenv venv shell %s\n", t.venvName)
		} else {
			fmt.Println("\nEnv vars updated. Add this to your shell rc if you haven't:")
			fmt.Println("  source ~/.config/coffeeenv/activate.sh")
		}
		return
	}
}
