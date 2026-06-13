// Identical to examples/claude-basic except the agent target — proving the
// skill / MCP / CLAUDE.md data is polymorphic across agents. The same `agent.*`
// data renders into Codex's layout (AGENTS.md + config.toml).
package env

import "coffeeenv.dev/lib/agent/codex"

#Main: {
	codex.#Main
	agent: md: project: "# Project\n\nManaged by coffeeenv."
	agent: skills: hello: {
		body: "---\nname: hello\ndescription: Say hello\n---\n\nGreet the user warmly."
	}
	agent: mcps: filesystem: {
		command: "npx"
		args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
	}
}

#Main
