// Full declarative coffeectx setup for Claude. Lists the projects to configure;
// each project's repoPath / embed / lsp / pi-extension / skills are prompted by
// `apply` (or supplied with --value 'projects[0].repoPath=...'). Generates
// ~/.coffeecode/config.yaml plus LSP install + pi-extension steps.
package env

import (
	"coffeeenv.dev/lib/agent"
	"coffeeenv.dev/lib/agent/claude"
	"coffeeenv.dev/lib/coffeectx"
)

// `projects` carries #Project's @input fields so the resolver discovers/prompts
// them per project. Project to a plain-struct list before handing to #Setup —
// passing #Project-typed elements through a definition param leaves a downstream
// dynamic-key map incomplete (a CUE evaluation quirk).
projects: [
	coffeectx.#Project & {name: "myrepo"},
]
_plain: [for p in projects {{
	name:          p.name
	repoPath:      p.repoPath
	embedProvider: p.embedProvider
	lspCommand:    p.lspCommand
	lspInstall:    p.lspInstall
	installPiExt:  p.installPiExt
	skills:        p.skills
}}]

states: (agent.#Render & {
	targets: [
		claude.#Claude,
		coffeectx.#Setup & {projects: _plain},
	]
}).states
