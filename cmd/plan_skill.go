package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	planSkillJSON   bool
	planSkillVenv   string
	planSkillValues []string
)

var planSkillCmd = &cobra.Command{
	Use:   "plan-skill <skill-source>",
	Short: "Show what adding a skill would change",
	Long: `Pull an agent skill from a git/oci/local source, add it to the
composition's skills, and show the actions needed to converge.

Modes:
  plan-skill <source>                against the global setup
  plan-skill --venv <name> <source>  into a local venv`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		values, err := parseValues(planSkillValues)
		if err != nil {
			return err
		}
		t, err := resolveSkillTarget(cmd.Context(), args[0], planSkillVenv, values)
		if err != nil {
			return err
		}
		p, _, err := computePlan(cmd.Context(), t, nil)
		if err != nil {
			return err
		}
		if planSkillJSON {
			return printPlanJSON(p)
		}
		fmt.Printf("Target: %s\n", t.label)
		if len(t.manifest.Execs) == 0 {
			fmt.Println("Note: no agent chart in this target — the skill won't render until an agent chart is applied.")
		}
		printPlan(p)
		return nil
	},
}

func init() {
	planSkillCmd.Flags().BoolVar(&planSkillJSON, "json", false, "emit the action list as JSON")
	planSkillCmd.Flags().StringVar(&planSkillVenv, "venv", "", "render into the named venv (engine=local)")
	planSkillCmd.Flags().StringArrayVarP(&planSkillValues, "value", "V", nil, "set an input value: key=val (repeatable)")
}
