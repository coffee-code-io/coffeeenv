package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	applySkillVenv   string
	applySkillValues []string
)

var applySkillCmd = &cobra.Command{
	Use:   "apply-skill <skill-source>",
	Short: "Pull a skill and add it to the agent",
	Long: `Pull an agent skill (agentskills.io / Anthropic Agent Skills format — a
directory with SKILL.md) from a git/oci/local source and add it to the
composition's skills. The active agent target renders it into its skills dir.

Modes:
  apply-skill <source>                add to the global setup
  apply-skill --venv <name> <source>  add into a local venv

Example:
  coffeeenv apply-skill git+https://github.com/anthropics/skills.git#main:skills/pdf`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		values, err := parseValues(applySkillValues)
		if err != nil {
			return err
		}
		t, err := resolveSkillTarget(cmd.Context(), args[0], applySkillVenv, values)
		if err != nil {
			return err
		}
		if len(t.manifest.Execs) == 0 {
			fmt.Println("Note: no agent chart applied yet — the skill is recorded but won't render until you apply an agent chart.")
		}
		return executeApply(cmd, t, true)
	},
}

func init() {
	applySkillCmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "apply without prompting")
	applySkillCmd.Flags().StringVar(&applySkillVenv, "venv", "", "add into the named venv (engine=local)")
	applySkillCmd.Flags().StringArrayVarP(&applySkillValues, "value", "V", nil, "set an input value: key=val (repeatable)")
}
