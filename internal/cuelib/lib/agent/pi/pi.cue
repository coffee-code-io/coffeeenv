// Package pi renders the shared agent namespace into pi.dev's layout under the
// pi agent dir (default ~/.pi/agent, overridable via PI_CODING_AGENT_DIR):
// AGENTS.md, skills/<name>/SKILL.md, mcp.json, and extensions/<name>.ts. pi has
// no native MCP support — when any MCP is configured we write mcp.json and add
// the `pi-mcp-adapter` pi package, which bridges those servers. pi packages
// (the adapter, plugins) install via `pi install npm:<package>`. #Main is a
// mixin: embed it and write `agent.*` data.
package pi

import (
	"strings"
	"list"
	"coffeeenv.dev/lib/context"
	core "coffeeenv.dev/lib/core"
	ag "coffeeenv.dev/lib/agent"
	st "coffeeenv.dev/lib/states"
)

// #Main is the pi.dev agent target.
#Main: {
	core.#Main
	agent: ag.#NS
	agent: name: "pi"

	_home:   context.root
	_local:  context.engine == "local"
	_mdKeys: list.SortStrings([for k, _ in agent.md {k}])
	// The pi agent dir holds all managed files; PI_CODING_AGENT_DIR points here
	// (it names the agent dir itself, not a parent), defaulting to ~/.pi/agent.
	_agentDir: "\(_home)/.pi/agent"

	// pi has no native MCP support: when any MCP is configured, bridge it via the
	// pi-mcp-adapter package (it reads the mcp.json written below). The adapter is
	// just another extension, installed by the package loop. The closed-empty
	// unification means "non-empty" without forcing an unresolved input.
	if (close({}) & agent.mcps) == _|_ {
		agent: extensions: "pi-mcp-adapter": {package: "pi-mcp-adapter"}
	}

	states: {
		"pi": st.#NpmState & {
			package: "@earendil-works/pi-coding-agent"
			version: agent.version
			if _local {prefix: context.root}
		}

		if len(agent.md) > 0 {
			"pi-agents": st.#FileState & {
				path:    "\(_agentDir)/AGENTS.md"
				content: strings.Join([for k in _mdKeys {agent.md[k]}], "\n\n")
			}
		}

		for sname, sk in agent.skills if sk.body != "" {
			"pi-skill-\(sname)": st.#FileState & {
				path:    "\(_agentDir)/skills/\(sname)/SKILL.md"
				content: sk.body
			}
		}
		for sname, sk in agent.skills if (sk.files & string) != _|_ {
			"pi-skill-\(sname)-files": st.#CopyState & {
				src: sk.files
				dst: "\(_agentDir)/skills/\(sname)"
			}
		}
		for sname, sk in agent.skills if (sk.files & {[string]: string}) != _|_ for fpath, fcontent in sk.files {
			"pi-skill-\(sname)-\(fpath)": st.#FileState & {
				path:    "\(_agentDir)/skills/\(sname)/\(fpath)"
				content: fcontent
			}
		}

		// mcp.json — read by pi-mcp-adapter, not by pi itself. Same closed-empty
		// gate as the adapter above so both appear together.
		if (close({}) & agent.mcps) == _|_ {
			"pi-mcp": st.#FileState & {
				path:   "\(_agentDir)/mcp.json"
				format: "json"
				data: mcpServers: {
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

		// Extensions: inline TS body, copied/inline files, and pi packages.
		for ename, ex in agent.extensions if ex.body != "" {
			"pi-extension-\(ename)": st.#FileState & {
				path:    "\(_agentDir)/extensions/\(ename).ts"
				content: ex.body
			}
		}
		for ename, ex in agent.extensions if (ex.files & string) != _|_ {
			"pi-extension-\(ename)-files": st.#CopyState & {
				src: ex.files
				dst: "\(_agentDir)/extensions/\(ename)"
			}
		}
		for ename, ex in agent.extensions if (ex.files & {[string]: string}) != _|_ for fpath, fcontent in ex.files {
			"pi-extension-\(ename)-\(fpath)": st.#FileState & {
				path:    "\(_agentDir)/extensions/\(ename)/\(fpath)"
				content: fcontent
			}
		}
		// A pi package installs via the CLI; guard with `pi list` for idempotency.
		for ename, ex in agent.extensions if ex.package != _|_ {
			"pi-package-\(ename)": st.#ShellState & {
				run:    "pi install npm:\(ex.package)"
				unless: "pi list | grep -q \(ex.package)"
			}
		}

		if _local {
			"PI_CODING_AGENT_DIR": st.#EnvState & {
				value:  _agentDir
				target: "\(context.root)/env.sh"
			}
		}
	}
}
