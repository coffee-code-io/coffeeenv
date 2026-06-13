// A minimal Claude Code chart. It defines an executable #Main (composed by
// `apply`) and also embeds it at the top level so the chart evaluates directly.
// The skill, MCP, and CLAUDE.md part are agent-agnostic — swap claude.#Main for
// codex.#Main (see examples/codex-basic) to render into Codex's layout.
package env

import "coffeeenv.dev/lib/agent/claude"

#Main: {
	claude.#Main
	agent: md: project: "# Project\n\nManaged by coffeeenv."
	agent: skills: hello: {
		body: "---\nname: hello\ndescription: Say hello\n---\n\nGreet the user warmly."
	}
	agent: mcps: filesystem: {
		command: "npx"
		args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
	}
}

#Main // expose at the top level for direct `plan`/`apply` and eval
