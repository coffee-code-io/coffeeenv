// Full declarative coffeectx setup for Claude. Lists the projects to configure;
// each project's repoPath / language / skills / jobs are prompted by `apply` (or
// supplied with --value 'projects[0].repoPath=...'). Generates
// ~/.coffeecode/config.yaml plus skill/job installs. The LSP server itself is
// installed by the standalone `lsp.#LSP` target, which reuses the project's
// resolved language so it isn't prompted twice.
package env

import (
	"coffeeenv.dev/lib/agent"
	"coffeeenv.dev/lib/agent/claude"
	"coffeeenv.dev/lib/lsp"
	"coffeeenv.dev/lib/coffeectx"
)

// Top-level fields carry the @input annotations the resolver prompts for (it
// only scans top-level fields, not the `states` output). `projects` carries
// #Project's per-project inputs; `input` carries the setup-wide config.
input: coffeectx.#SetupInput

// `projects` carries #Project's @input fields so the resolver discovers/prompts
// them per project. Project to a plain-struct list before handing to #Setup —
// passing #Project-typed elements through a definition param leaves a downstream
// dynamic-key map incomplete (a CUE evaluation quirk).
projects: [
	coffeectx.#Project & {name: "myrepo"},
]
_plain: [for p in projects {{
	name:     p.name
	repoPath: p.repoPath
	language: p.language
	skills:   p.skills
	jobs:     p.jobs
}}]

states: (agent.#Render & {
	targets: [
		claude.#Claude,
		// Install the LSP server for the project's language (reuses the resolved
		// project language; no separate prompt).
		lsp.#LSP & {language: _plain[0].language},
		// A coffeectx job, registered the same way skills are.
		coffeectx.#RegisterJob & {job: {
			name: "reindex"
			body: "---\nname: reindex\ndescription: Re-index the repo\n---\n\nRebuild the knowledge graph."
		}},
		coffeectx.#Setup & {"input": input, projects: _plain},
	]
}).states
