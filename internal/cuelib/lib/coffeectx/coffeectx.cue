// Package coffeectx installs and configures the coffeectx knowledge-graph
// toolchain for the active agent. #Install is the no-prompt module (npm + MCP
// registration + the explanation paragraph). #Setup is the full declarative
// configuration: it generates ~/.coffeecode/config.yaml, LSP install steps, and
// the pi extension from a list of projects whose per-project fields are prompted.
package coffeectx

import (
	"strings"
	"coffeeenv.dev/lib/context"
	"coffeeenv.dev/lib/agent"
	st "coffeeenv.dev/lib/states"
)

// _explain is the CoffeeCtx paragraph appended to the agent's AGENTS.md/CLAUDE.md.
_explain: """
	## CoffeeCtx

	CoffeeCtx is a knowledge graph MCP server that indexes your codebase, agent logs, and
	architecture decisions so you can query them with semantic and structural search.

	Available MCP tools: `search` (semantic similarity), `exact` (exact symbol match),
	`regex` (regex over symbols), `raw_query` (graph query language), `load_node`.

	Use it when unsure whether an implementation is temporary vs. a deliberate decision, or
	to recall why one approach was chosen over another.
	"""

// _mcpEntry registers the coffeectx MCP server under the agent. The server bin is
// `coffeectx-mcp` (from @coffeectx/server); a venv install puts it on PATH via
// node_modules/.bin.
_mcpEntry: {name: "coffeectx", command: "coffeectx-mcp"}

// #Install — install only coffeectx for the active agent. No prompts.
#Install: agent.#Target & {
	version: string | *"latest"
	explain: bool | *true
	_local:  context.engine == "local"

	register: {
		mcps: coffeectx: _mcpEntry
		if explain {
			agentMd: [_explain]
		}
	}
	states: [
		st.#NpmState & {
			name:    "coffeectx-server"
			package: "@coffeectx/server"
			version: version
			if _local {prefix: context.root}
		},
	]
}

// #Project describes one coffeectx project. `name` is supplied by the chart;
// every other field carries @input and is prompted per project (no defaults, so
// they stay non-concrete until resolved).
#Project: {
	name:         string
	repoPath:     string @input("Repo path", order=1)
	embedProvider: string @input("Embed provider (stub/openai/ollama)", order=2)
	lspCommand:   string @input("LSP command (empty for none)", order=3)
	lspInstall:   string @input("LSP install command (empty to skip)", order=4)
	installPiExt: bool   @input("Install the pi.dev extension? (true/false)", order=5)
	skills:       string @input("Skills to enable, comma-separated (empty for none)", order=6)
}

// #Setup — full declarative coffeectx configuration for a list of projects.
#Setup: agent.#Target & {
	version: string | *"latest"
	// Untyped so `& {projects: chartProjects}` doesn't create a separate
	// unification node that FillPath (applied to the chart field) can't reach.
	// The chart supplies #Project-typed elements.
	projects: [...]
	_local: context.engine == "local"

	// Project a plain-struct list first. A dynamic-key map comprehension
	// (`(p.name): …`) directly over the #Project-typed list yields an
	// incomplete value; projecting to plain structs via a list comprehension
	// first sidesteps that CUE evaluation quirk.
	_plain: [for p in projects {{
		name:          p.name
		repoPath:      p.repoPath
		embedProvider: p.embedProvider
		lspCommand:    p.lspCommand
		lspInstall:    p.lspInstall
		installPiExt:  p.installPiExt
		skills:        p.skills
	}}]

	// ~/.coffeecode/config.yaml mirrored from the CoffeectxConfig schema.
	_config: {
		projects: {
			for p in _plain {
				(p.name): {
					db:       "~/.coffeecode/db/\(p.name).db"
					repoPath: p.repoPath
					enabled:  true
					core: embed: provider: p.embedProvider
					mcp: tools: {search: true, exact: true, regex: true, raw_query: true, load_node: true, insert: false}
					if p.lspCommand != "" {
						jobs: lsp: {enabled: true, parameters: lspCommand: p.lspCommand}
					}
					if p.skills != "" {
						skills: jobs: include: strings.Split(p.skills, ",")
					}
				}
			}
		}
		types: userDir: "~/.coffeecode/types"
	}

	_anyPi: len([for p in _plain if p.installPiExt {p}]) > 0
	_lspInstall: [for p in _plain if p.lspInstall != "" if len(strings.Fields(p.lspCommand)) > 0 {p}]

	register: {
		mcps: coffeectx: _mcpEntry
		agentMd: [_explain]
	}

	states: [
		st.#FileState & {
			name:   "coffeecode-config"
			path:   "~/.coffeecode/config.yaml"
			format: "yaml"
			data:   _config
		},
		st.#NpmState & {
			name:    "coffeectx-server"
			package: "@coffeectx/server"
			version: version
			if _local {prefix: context.root}
		},
		for p in _lspInstall {
			st.#ShellState & {
				name:   "lsp-install-\(p.name)"
				run:    p.lspInstall
				unless: "command -v \(strings.Fields(p.lspCommand)[0]) >/dev/null 2>&1"
			}
		},
		if _anyPi {
			st.#NpmState & {
				name:    "coffeectx-pi-plugin"
				package: "@coffeectx/pi-plugin"
				version: version
				if _local {prefix: context.root}
			}
		},
		if _anyPi {
			st.#FileState & {
				name:    "coffeectx-pi-ext"
				path:    "~/.pi/agent/extensions/coffeectx.ts"
				content: "export { default } from '@coffeectx/pi-plugin';\n"
			}
		},
	]
}
