// Package codex renders the shared agent namespace into OpenAI Codex's layout:
// ~/.codex/AGENTS.md and ~/.codex/config.toml. Codex has no native skill
// concept, so skills with an inline body fold into AGENTS.md as sections.
// #Codex is a mixin: embed it and write `agent.*` data.
package codex

import (
	"strings"
	"list"
	"coffeeenv.dev/lib/context"
	core "coffeeenv.dev/lib/core"
	ag "coffeeenv.dev/lib/agent"
	st "coffeeenv.dev/lib/states"
)

// #Codex is the OpenAI Codex agent target.
#Main: {
	core.#Main
	agent: ag.#NS
	agent: name: "codex"

	_home:   context.root
	_local:  context.engine == "local"
	_mdKeys: list.SortStrings([for k, _ in agent.md {k}])

	// AGENTS.md = md parts (sorted) followed by each inline-body skill as a
	// "## Skill: <name>" section (sorted by skill name).
	_skillKeys: list.SortStrings([for k, sk in agent.skills if sk.body != "" {k}])
	_parts: [for k in _mdKeys {agent.md[k]}] +
		[for k in _skillKeys {"## Skill: \(k)\n\n\(agent.skills[k].body)"}]

	states: {
		"codex": st.#NpmState & {
			package: "@openai/codex"
			version: agent.version
			if _local {prefix: context.root}
		}

		if len(_parts) > 0 {
			"codex-agents": st.#FileState & {
				path:    "\(_home)/.codex/AGENTS.md"
				content: strings.Join(_parts, "\n\n")
			}
		}

		// Emptiness via unification with a closed empty struct (not len): keeps the
		// build from forcing an input a feature gates its MCP contribution on.
		if (close({}) & agent.mcps) == _|_ {
			"codex-mcp": st.#FileState & {
				path:   "\(_home)/.codex/config.toml"
				format: "toml"
				data: mcp_servers: {
					for mname, m in agent.mcps {
						(mname): {
							if m.command != _|_ {command: m.command}
							if m.args != _|_ {args: m.args}
							if m.env != _|_ {env: m.env}
							if m.url != _|_ {url: m.url}
							if m.transport != _|_ {transport: m.transport}
						}
					}
				}
			}
		}

		if _local {
			"CODEX_HOME": st.#EnvState & {
				value:  "\(context.root)/.codex"
				target: "\(context.root)/env.sh"
			}
		}
	}
}
