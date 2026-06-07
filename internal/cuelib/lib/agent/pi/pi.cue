// Package pi renders the shared agent Context into pi.dev's layout under
// ~/.pi/agent: AGENTS.md, skills/<name>/SKILL.md, and mcp.json. pi.dev's exact
// package name and config paths should be confirmed against the real CLI; the
// ~/.pi/agent base matches the extension path coffeectx already targets
// (~/.pi/agent/extensions/coffeectx.ts).
package pi

import (
	"strings"
	"coffeeenv.dev/lib/context"
	"coffeeenv.dev/lib/agent"
	st "coffeeenv.dev/lib/states"
)

// #Pi is an agent target: place it in a chart's targets list.
#Pi: agent.#Target & {
	// ctx is provided by agent.#Render; redeclared here so bare `ctx` references
	// below resolve lexically within this conjunct.
	ctx: agent.#Context

	version: string | *"latest"

	// Announce the active agent so agent-agnostic libraries can branch on it.
	register: agent: "pi"

	// _home is "~" (global) or the venv root (local).
	_home:  context.root
	_local: context.engine == "local"

	states: [
		st.#NpmState & {
			name:    "pi-cli"
			package: "@pi-dev/cli"
			version: version
			if _local {prefix: context.root}
		},
		if len(ctx.agentMd) > 0 {
			st.#FileState & {
				name:    "pi-agents"
				path:    "\(_home)/.pi/agent/AGENTS.md"
				content: strings.Join(ctx.agentMd, "\n\n")
			}
		},
		for sname, sk in ctx.skills {
			st.#FileState & {
				name:    "pi-skill-\(sname)"
				path:    "\(_home)/.pi/agent/skills/\(sname)/SKILL.md"
				content: sk.body
			}
		},
		for sname, sk in ctx.skills for fpath, fcontent in sk.files {
			st.#FileState & {
				name:    "pi-skill-\(sname)-file"
				path:    "\(_home)/.pi/agent/skills/\(sname)/\(fpath)"
				content: fcontent
			}
		},
		if len(ctx.mcps) > 0 {
			st.#FileState & {
				name:   "pi-mcp"
				path:   "\(_home)/.pi/agent/mcp.json"
				format: "json"
				data: mcpServers: {
					for mname, m in ctx.mcps {
						(mname): {
							if m.command != _|_ {command: m.command}
							if m.args != _|_ {args: m.args}
							if m.env != _|_ {env: m.env}
							if m.url != _|_ {url: m.url}
						}
					}
				}
			}
		},
		if _local {
			st.#EnvState & {
				name:   "PI_HOME"
				value:  "\(context.root)/.pi"
				target: "\(context.root)/env.sh"
			}
		},
	]
}
