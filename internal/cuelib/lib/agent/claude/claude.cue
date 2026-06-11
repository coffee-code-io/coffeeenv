// Package claude renders the shared agent namespace into Claude Code's layout:
// ~/.claude/CLAUDE.md, ~/.claude/skills/<name>/SKILL.md, and ~/.claude.json.
// #Claude is a mixin: embed it at the chart's top level and write `agent.*`
// data; it reads that data and contributes the file/install states.
package claude

import (
	"strings"
	"list"
	"coffeeenv.dev/lib/context"
	ag "coffeeenv.dev/lib/agent"
	st "coffeeenv.dev/lib/states"
)

// #Claude is the Claude Code agent target.
#Claude: {
	ag.#Base
	agent: ag.#NS
	agent: name: "claude"

	_home:   context.root
	_local:  context.engine == "local"
	_mdKeys: list.SortStrings([for k, _ in agent.md {k}])

	states: {
		"claude-code": st.#NpmState & {
			package: "@anthropic-ai/claude-code"
			version: agent.version
			if _local {prefix: context.root}
		}

		if len(agent.md) > 0 {
			"claude-claudemd": st.#FileState & {
				path:    "\(_home)/.claude/CLAUDE.md"
				content: strings.Join([for k in _mdKeys {agent.md[k]}], "\n\n")
			}
		}

		// Skill with an inline body -> SKILL.md.
		for sname, sk in agent.skills if sk.body != "" {
			"claude-skill-\(sname)": st.#FileState & {
				path:    "\(_home)/.claude/skills/\(sname)/SKILL.md"
				content: sk.body
			}
		}
		// Skill backed by a filesystem path -> copy the tree in.
		for sname, sk in agent.skills if (sk.files & string) != _|_ {
			"claude-skill-\(sname)-files": st.#CopyState & {
				src: sk.files
				dst: "\(_home)/.claude/skills/\(sname)"
			}
		}
		// Skill with inline extra files (relpath -> content).
		for sname, sk in agent.skills if (sk.files & {[string]: string}) != _|_ for fpath, fcontent in sk.files {
			"claude-skill-\(sname)-\(fpath)": st.#FileState & {
				path:    "\(_home)/.claude/skills/\(sname)/\(fpath)"
				content: fcontent
			}
		}

		// Emptiness via unification with a closed empty struct (not len): when a
		// feature gates its MCP contribution on an unresolved input, this stays
		// incomplete (deferred) rather than forcing that input at build time.
		if (close({}) & agent.mcps) == _|_ {
			"claude-mcp": st.#FileState & {
				path:   "\(_home)/.claude.json"
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
			"CLAUDE_CONFIG_DIR": st.#EnvState & {
				value:  "\(context.root)/.claude"
				target: "\(context.root)/env.sh"
			}
		}
	}
}
