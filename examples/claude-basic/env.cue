// A minimal Claude Code chart on the polymorphic framework. The skill, MCP, and
// AGENTS.md parts are agent-agnostic — swap claude.#Claude for codex.#Codex
// (see examples/codex-basic) and they render into Codex's layout instead.
package env

import (
	"coffeeenv.dev/lib/agent"
	"coffeeenv.dev/lib/agent/claude"
)

states: (agent.#Render & {
	targets: [
		claude.#Claude,
		agent.#RegisterAgentMd & {text: "# Project\n\nManaged by coffeeenv."},
		agent.#RegisterSkill & {skill: {
			name: "hello"
			body: "---\nname: hello\ndescription: Say hello\n---\n\nGreet the user warmly."
		}},
		agent.#RegisterMCP & {mcp: {
			name:    "filesystem"
			command: "npx"
			args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
		}},
	]
}).states
