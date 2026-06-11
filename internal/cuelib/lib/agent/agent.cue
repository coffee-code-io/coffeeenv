// Package agent is the polymorphic core, redesigned around shared global state.
// Instead of piping a list of targets through a render function, every target is
// a mixin embedded at the chart's top level. Targets read and write shared
// namespaces — the agent namespace (`agent.skills`, `agent.mcps`, `agent.md`,
// `agent.name`) and the open `states` map — and references inside a mixin
// re-resolve against the unified top-level value once embedded. A chart adds a
// skill by writing data:
//
//	agent: skills: hello: {body: "..."}
//
// and the active agent target (claude/codex/pi) reads `agent.skills` to render
// it into that agent's file layout.
package agent

import (
	"coffeeenv.dev/lib/context"
	st "coffeeenv.dev/lib/states"
)

// #Skill is an agent-agnostic skill. Content comes from either an inline `body`
// (becomes SKILL.md) or `files`: a filesystem path (string — copied in via a
// #CopyState) or an inline map of relative-path -> content. Both default empty,
// so renderers can branch with `if sk.body != ""` / a string-vs-map check
// without tripping over absent optional fields.
#Skill: {
	name?:        string
	description?: string
	body:         string | *""
	files:        string | {[string]: string} | *{}
}

// #MCP is an agent-agnostic MCP server spec (command- or url-based).
#MCP: {
	name?: string
	command?: string
	args?: [...string]
	env?: {[string]: string}
	url?: string
}

// #NS is the schema of the shared `agent` namespace. Every target that reads or
// writes the namespace declares `agent: #NS` (an explicit field, so bare
// `agent` references inside the target's comprehensions resolve after the mixin
// is embedded). `name` is set by the active agent target; `md` is a name-keyed
// map of AGENTS.md/CLAUDE.md paragraphs joined in sorted-key order.
#NS: {
	name:    string | *""
	version: string | *"latest"
	skills: {[string]: #Skill}
	mcps: {[string]: #MCP}
	md: {[string]: string}
}

// #Base types the global output (`states` is an open map of basic states) and
// adds the venv PATH state. Agent targets embed it; feature mixins only need to
// type `states` (which unifies with this). Under the local engine, local npm
// installs land bins in <root>/node_modules/.bin — put that on PATH.
#Base: {
	states: {[string]: st.#State}
	if context.engine == "local" {
		states: "PATH": st.#EnvState & {
			value:  "\(context.root)/node_modules/.bin:$PATH"
			expand: true
			target: "\(context.root)/env.sh"
		}
	}
}
