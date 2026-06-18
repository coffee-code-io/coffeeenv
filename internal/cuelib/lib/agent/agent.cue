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

// #PiExtension is a pi.dev integration (only the pi target renders these — it is
// defined here, alongside #Skill/#MCP, to keep the shared #NS schema in one place
// and avoid a pi->agent import cycle). It can be any of: a pi `package` installed
// via `pi install npm:<package>`; a TS module from an inline `body` (->
// extensions/<name>.ts); or `files` (a filesystem path copied in, or an inline
// relpath -> content map under extensions/<name>/).
#PiExtension: {
	name?:        string
	description?: string
	package?:     string
	body:         string | *""
	files:        string | {[string]: string} | *{}
}

// #NS is the schema of the shared `agent` namespace. Every target that reads or
// writes the namespace declares `agent: #NS` (an explicit field, so bare
// `agent` references inside the target's comprehensions resolve after the mixin
// is embedded). `name` is set by the active agent target; `md` is a name-keyed
// map of AGENTS.md/CLAUDE.md paragraphs joined in sorted-key order; `extensions`
// is read only by the pi target (claude/codex ignore it).
#NS: {
	name:    string | *""
	version: string | *"latest"
	skills: {[string]: #Skill}
	mcps: {[string]: #MCP}
	md: {[string]: string}
	extensions: {[string]: #PiExtension}
}

