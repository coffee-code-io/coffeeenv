// Package claude renders the shared agent Context into Claude Code's layout:
// ~/.claude/CLAUDE.md, ~/.claude/skills/<name>/SKILL.md, and ~/.claude.json.
package claude

import (
	"strings"
	"coffeeenv.dev/lib/context"
	"coffeeenv.dev/lib/agent"
	st "coffeeenv.dev/lib/states"
)

// #Claude is an agent target: place it in a chart's targets list.
#Claude: agent.#Target & {
	// ctx is provided by agent.#Render; redeclared here so bare `ctx` references
	// below resolve lexically within this conjunct.
	ctx: agent.#Context

	version: string | *"latest"

	// _home is "~" (global) or the venv root (local).
	_home:  context.root
	_local: context.engine == "local"

	states: [
		st.#NpmState & {
			name:    "claude-code"
			package: "@anthropic-ai/claude-code"
			version: version
			if _local {prefix: context.root}
		},
		if len(ctx.agentMd) > 0 {
			st.#FileState & {
				name:    "claude-claudemd"
				path:    "\(_home)/.claude/CLAUDE.md"
				content: strings.Join(ctx.agentMd, "\n\n")
			}
		},
		for sname, sk in ctx.skills {
			st.#FileState & {
				name:    "claude-skill-\(sname)"
				path:    "\(_home)/.claude/skills/\(sname)/SKILL.md"
				content: sk.body
			}
		},
		for sname, sk in ctx.skills for fpath, fcontent in sk.files {
			st.#FileState & {
				name:    "claude-skill-\(sname)-file"
				path:    "\(_home)/.claude/skills/\(sname)/\(fpath)"
				content: fcontent
			}
		},
		if len(ctx.mcps) > 0 {
			st.#FileState & {
				name:   "claude-mcp"
				path:   "\(_home)/.claude.json"
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
				name:   "CLAUDE_CONFIG_DIR"
				value:  "\(context.root)/.claude"
				target: "\(context.root)/env.sh"
			}
		},
	]
}
