// Claude + Coffeecode: a minimal chart that installs coffeectx for Claude Code.
// #Main composes the Claude agent target and coffeectx.#Mcp; coffeectx feeds the
// MCP server into agent.mcps, which the Claude target renders into ~/.claude.json.
// (Swap claude.#Main for codex.#Main, or pi.#Main to install the pi.dev extension.)
package env

import (
	"coffeeenv.dev/lib/agent/claude"
	"coffeeenv.dev/lib/coffeectx"
)

#Main: {
	claude.#Main
	coffeectx.#Mcp
}

#Main
