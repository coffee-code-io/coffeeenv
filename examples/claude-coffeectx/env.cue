// Claude + Coffeecode: a minimal chart that installs coffeectx for Claude Code.
// Embed the agent target and coffeectx.#Mcp; the only prompt is
// `coffeectx.confirm`. coffeectx feeds the MCP server into agent.mcps, which the
// Claude target renders into ~/.claude.json. (Swap claude.#Claude for
// codex.#Codex to target Codex, or pi.#Pi to install the pi.dev extension.)
package env

import (
	"coffeeenv.dev/lib/agent/claude"
	"coffeeenv.dev/lib/coffeectx"
)

claude.#Claude
coffeectx.#Mcp
