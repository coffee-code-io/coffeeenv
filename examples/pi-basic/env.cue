// Minimal pi.dev chart: installs the pi coding agent and exercises the shared
// agent namespace — a skill, an MCP server (bridged via pi-mcp-adapter, since pi
// has no native MCP), and a pi extension. Swap pi.#Main for claude.#Main or
// codex.#Main to target a different agent.
package env

import "coffeeenv.dev/lib/agent/pi"

#Main: {
	pi.#Main

	// A skill (renders to <agent dir>/skills/hello/SKILL.md).
	agent: skills: hello: {body: "---\nname: hello\ndescription: Say hi\n---\n\nGreet the user."}

	// An MCP server. pi has no native MCP, so this drives mcp.json + the
	// auto-added pi-mcp-adapter package.
	agent: mcps: example: {command: "example-mcp", args: ["--stdio"]}

	// A pi extension (renders to <agent dir>/extensions/greet.ts).
	agent: extensions: greet: {
		body: "import type { ExtensionAPI } from \"@earendil-works/pi-coding-agent\";\nexport default function (pi: ExtensionAPI) {}\n"
	}
}

#Main
