// Package codex renders the shared agent Context into OpenAI Codex's layout:
// ~/.codex/AGENTS.md and ~/.codex/config.toml. Codex has no native skill
// concept, so skills fold into AGENTS.md as sections.
package codex

import (
	"strings"
	"coffeeenv.dev/lib/context"
	"coffeeenv.dev/lib/agent"
	st "coffeeenv.dev/lib/states"
)

// #Codex is an agent target: place it in a chart's targets list.
#Codex: agent.#Target & {
	// ctx is provided by agent.#Render; redeclared here so bare `ctx` references
	// below resolve lexically within this conjunct.
	ctx: agent.#Context

	version: string | *"latest"

	_home:  context.root
	_local: context.engine == "local"

	// AGENTS.md = agentMd parts + each skill as a "## Skill: <name>" section.
	_parts: [for p in ctx.agentMd {p}] +
		[for sname, sk in ctx.skills {"## Skill: \(sname)\n\n\(sk.body)"}]

	states: [
		st.#NpmState & {
			name:    "codex"
			package: "@openai/codex"
			version: version
			if _local {prefix: context.root}
		},
		if len(_parts) > 0 {
			st.#FileState & {
				name:    "codex-agents"
				path:    "\(_home)/.codex/AGENTS.md"
				content: strings.Join(_parts, "\n\n")
			}
		},
		if len(ctx.mcps) > 0 {
			st.#FileState & {
				name:   "codex-mcp"
				path:   "\(_home)/.codex/config.toml"
				format: "toml"
				data: mcp_servers: {
					for mname, m in ctx.mcps {
						(mname): {
							if m.command != _|_ {command: m.command}
							if m.args != _|_ {args: m.args}
							if m.env != _|_ {env: m.env}
						}
					}
				}
			}
		},
		if _local {
			st.#EnvState & {
				name:   "CODEX_HOME"
				value:  "\(context.root)/.codex"
				target: "\(context.root)/env.sh"
			}
		},
	]
}
