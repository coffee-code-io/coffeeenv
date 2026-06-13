// Package pi renders the shared agent namespace into pi.dev's layout under
// ~/.pi/agent: AGENTS.md, skills/<name>/SKILL.md, and mcp.json. pi.dev's exact
// package name and config paths should be confirmed against the real CLI; the
// ~/.pi/agent base matches the extension path coffeectx already targets
// (~/.pi/agent/extensions/coffeectx.ts). #Pi is a mixin: embed it and write
// `agent.*` data.
package pi

import (
	"strings"
	"list"
	"coffeeenv.dev/lib/context"
	core "coffeeenv.dev/lib/core"
	ag "coffeeenv.dev/lib/agent"
	st "coffeeenv.dev/lib/states"
)

// #Pi is the pi.dev agent target.
#Main: {
	core.#Main
	agent: ag.#NS
	agent: name: "pi"

	_home:   context.root
	_local:  context.engine == "local"
	_mdKeys: list.SortStrings([for k, _ in agent.md {k}])

	states: {
		"pi-cli": st.#NpmState & {
			package: "@pi-dev/cli"
			version: agent.version
			if _local {prefix: context.root}
		}

		if len(agent.md) > 0 {
			"pi-agents": st.#FileState & {
				path:    "\(_home)/.pi/agent/AGENTS.md"
				content: strings.Join([for k in _mdKeys {agent.md[k]}], "\n\n")
			}
		}

		for sname, sk in agent.skills if sk.body != "" {
			"pi-skill-\(sname)": st.#FileState & {
				path:    "\(_home)/.pi/agent/skills/\(sname)/SKILL.md"
				content: sk.body
			}
		}
		for sname, sk in agent.skills if (sk.files & string) != _|_ {
			"pi-skill-\(sname)-files": st.#CopyState & {
				src: sk.files
				dst: "\(_home)/.pi/agent/skills/\(sname)"
			}
		}
		for sname, sk in agent.skills if (sk.files & {[string]: string}) != _|_ for fpath, fcontent in sk.files {
			"pi-skill-\(sname)-\(fpath)": st.#FileState & {
				path:    "\(_home)/.pi/agent/skills/\(sname)/\(fpath)"
				content: fcontent
			}
		}

		// Emptiness via unification with a closed empty struct (not len): keeps the
		// build from forcing an input a feature gates its MCP contribution on.
		if (close({}) & agent.mcps) == _|_ {
			"pi-mcp": st.#FileState & {
				path:   "\(_home)/.pi/agent/mcp.json"
				format: "json"
				data: mcpServers: {
					for mname, m in agent.mcps {
						(mname): {
							if m.command != _|_ {command: m.command}
							if m.args != _|_ {args: m.args}
							if m.env != _|_ {env: m.env}
							if m.url != _|_ {url: m.url}
						}
					}
				}
			}
		}

		if _local {
			"PI_HOME": st.#EnvState & {
				value:  "\(context.root)/.pi"
				target: "\(context.root)/env.sh"
			}
		}
	}
}
