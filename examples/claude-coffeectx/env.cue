// Claude + Coffeecode: a minimal chart that installs only coffeectx for Claude
// Code, with no prompts. (Swap claude.#Claude for codex.#Codex to target Codex.)
package env

import (
	"coffeeenv.dev/lib/agent"
	"coffeeenv.dev/lib/agent/claude"
	"coffeeenv.dev/lib/coffeectx"
)

states: (agent.#Render & {
	targets: [
		claude.#Claude,
		coffeectx.#Install,
	]
}).states
