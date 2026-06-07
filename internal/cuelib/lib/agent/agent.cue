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

// #Context is the shared registry that agents consume. `agent` is the active
// agent's name, set by the agent target (claude/codex/pi). The struct is open
// (`...`) so libraries can attach their own namespaced sections — e.g. coffeectx
// adds `coffeectx: {jobs: ...}` that flows through #Render like skills do.
#Context: {
	agent: string | *""
	skills: {[string]: #Skill}
	mcps: {[string]: #MCP}
	agentMd: [...string]
	...
}

// #Target is one list element. `ctx` is injected by #Render with the merged
// Context; `register` is this target's contribution; `states` are emitted
// directly (e.g. npm installs).
#Target: {
	ctx: #Context
	register: {
		// agent is set only by agent targets (claude/codex/pi).
		agent?: string
		skills: {[string]: #Skill} | *{}
		mcps: {[string]: #MCP} | *{}
		// Disjunction default: a bare [] would conflict-on-unify with [x].
		agentMd: [...string] | *[]
		// Open: libraries register extra namespaced sections (e.g. coffeectx.jobs)
		// that #Render deep-merges generically into the Context.
		...
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

	// The active agent name is static (set by an agent target's register.agent),
	// independent of ctx — compute it first so registrations can branch on it.
	_agent: [for t in targets if t.register.agent != _|_ {t.register.agent}, ""][0]

	// Inject the agent into each target's ctx before reading its register, so
	// register fields that branch on ctx.agent (e.g. coffeectx's MCP-vs-pi
	// decision) see the real agent rather than the default "". register.agent is
	// static, so this introduces no cycle.
	_withAgent: [for t in targets {t & {ctx: agent: _agent}}]

	_ctx: #Context & {
		agent: _agent
		skills: {for t in _withAgent {t.register.skills}}
		mcps: {for t in _withAgent {t.register.mcps}}
		agentMd: [for t in _withAgent for p in t.register.agentMd {p}]

		// Generic extension merge: embed every extra register section (anything
		// other than the known fields) so library namespaces like coffeectx.jobs
		// deep-merge across targets by unification, the same way skills do.
		for t in _withAgent {
			{for k, v in t.register if k != "agent" if k != "skills" if k != "mcps" if k != "agentMd" {(k): v}}
		}
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
