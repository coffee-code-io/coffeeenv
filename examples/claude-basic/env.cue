// A minimal Claude Code chart on the global-state framework. Embed the agent
// target (claude.#Claude) and write data into the shared `agent` namespace — the
// skill, MCP, and CLAUDE.md part are agent-agnostic, so swapping claude.#Claude
// for codex.#Codex (see examples/codex-basic) renders them into Codex's layout.
package env

import "coffeeenv.dev/lib/agent/claude"

claude.#Claude

agent: md: project: "# Project\n\nManaged by coffeeenv."
agent: skills: hello: {
	body: "---\nname: hello\ndescription: Say hello\n---\n\nGreet the user warmly."
}
agent: mcps: filesystem: {
	command: "npx"
	args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
}
