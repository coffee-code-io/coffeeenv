// Package agent is the polymorphic core: agent-agnostic features (skills, MCP
// servers, AGENTS.md parts) register into a shared #Context, and an agent target
// (claude.#Claude, codex.#Codex) consumes that Context and renders it into its
// own file layout. The chart lists a flat set of targets; #Render merges their
// registrations, injects the Context into each, and concatenates their states.
package agent

import (
	"coffeeenv.dev/lib/context"
	st "coffeeenv.dev/lib/states"
)

// #Skill is an agent-agnostic skill: a name, a markdown body, and optional extra
// files keyed by relative path.
#Skill: {
	name:        string
	description: string | *""
	body:        string
	files: {[string]: string} | *{}
}

// #MCP is an agent-agnostic MCP server spec (command- or url-based).
#MCP: {
	name: string
	command?: string
	args?: [...string]
	env?: {[string]: string}
	url?: string
}

// #Context is the shared registry that agents consume.
#Context: {
	skills: {[string]: #Skill}
	mcps: {[string]: #MCP}
	agentMd: [...string]
}

// #Target is one list element. `ctx` is injected by #Render with the merged
// Context; `register` is this target's contribution; `states` are emitted
// directly (e.g. npm installs).
#Target: {
	ctx: #Context
	register: {
		skills: {[string]: #Skill} | *{}
		mcps: {[string]: #MCP} | *{}
		// Disjunction default: a bare [] would conflict-on-unify with [x].
		agentMd: [...string] | *[]
	}
	states: [...st.#State] | *[]
	// Open: agent targets and register helpers add their own fields
	// (version, skill, mcp, text, ...).
	...
}

// #Render merges every target's registration into one Context, injects it into
// each target, and concatenates the resulting states. No cycle: _ctx depends
// only on each target's register (static), never on ctx.
#Render: {
	targets: [...#Target]
	_ctx: #Context & {
		skills: {for t in targets {t.register.skills}}
		mcps: {for t in targets {t.register.mcps}}
		agentMd: [for t in targets for p in t.register.agentMd {p}]
	}
	_targetStates: [for t in targets for s in (t & {ctx: _ctx}).states {s}]

	// When installing into a venv (local engine), local npm installs land bins in
	// <root>/node_modules/.bin — put that on PATH by default.
	_pathStates: [
		if context.engine == "local" {
			st.#EnvState & {
				name:   "PATH"
				value:  "\(context.root)/node_modules/.bin:$PATH"
				expand: true
				target: "\(context.root)/env.sh"
			}
		},
	]

	states: _targetStates + _pathStates
}

// Self-registering helpers. Each copies a whole sub-object into `register` to
// avoid optional-field self-reference traps.
#RegisterSkill: #Target & {
	skill: #Skill
	register: skills: (skill.name): skill
}
#RegisterMCP: #Target & {
	mcp: #MCP
	register: mcps: (mcp.name): mcp
}
#RegisterAgentMd: #Target & {
	text: string
	register: agentMd: [text]
}
