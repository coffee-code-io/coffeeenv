package cmd

import (
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"

	"github.com/coffee-code-io/coffeeenv/internal/cuelib"
)

// interactivePrompt is the cuelib.PromptFunc backed by survey: an arrow-key
// Select for @choose, a space-toggle MultiSelect for @multichoice, a yes/no
// Confirm for KindConfirm, and a plain Input for free text. A small fixed option
// set on a text input is shown inline as "(a/b/c)".
func interactivePrompt(in cuelib.Input) (string, error) {
	switch in.Kind {
	case cuelib.KindChoose:
		var out string
		err := survey.AskOne(&survey.Select{
			Message: in.Prompt,
			Options: in.Options,
		}, &out)
		return out, err

	case cuelib.KindMultichoice:
		var out []string
		err := survey.AskOne(&survey.MultiSelect{
			Message: in.Prompt,
			Options: in.Options,
		}, &out)
		if err != nil {
			return "", err
		}
		return strings.Join(out, ","), nil

	case cuelib.KindConfirm:
		var ok bool
		if err := survey.AskOne(&survey.Confirm{Message: in.Prompt}, &ok); err != nil {
			return "", err
		}
		if ok {
			return "y", nil
		}
		return "n", nil

	default: // KindText
		msg := in.Prompt
		if len(in.Options) > 0 {
			msg = fmt.Sprintf("%s (%s)", in.Prompt, strings.Join(in.Options, "/"))
		}
		var out string
		err := survey.AskOne(&survey.Input{Message: msg}, &out)
		return strings.TrimSpace(out), err
	}
}

// confirm asks a yes/no question via survey (used for the apply approval).
func confirm(prompt string) (bool, error) {
	var ok bool
	err := survey.AskOne(&survey.Confirm{Message: prompt}, &ok)
	return ok, err
}
