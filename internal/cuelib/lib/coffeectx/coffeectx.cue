// Package coffeectx installs and configures the coffeectx knowledge-graph
// toolchain for the active agent. #Install is the minimal module (npm + the
// agent integration + the explanation paragraph). #Setup is the full
// declarative configuration: it generates ~/.coffeecode/config.yaml, installs
// context skills and coffeectx jobs into ~/.coffeecode, and (on global installs)
// can register the daemon for auto-launch.
//
// The agent integration is driven by the active agent (ctx.agent, set by the
// agent target): for "pi" we install the pi.dev extension, otherwise we register
// the MCP server. Either way the install is gated behind a confirmation prompt.
package coffeectx

import (
	"strings"
	"coffeeenv.dev/lib/context"
	"coffeeenv.dev/lib/agent"
	// Aliased: the generated config has a `jobs.lsp` field that would otherwise
	// shadow this package name where the command is resolved.
	lsplib "coffeeenv.dev/lib/lsp"
	st "coffeeenv.dev/lib/states"
)

// _explain is the CoffeeCtx paragraph appended to the agent's AGENTS.md/CLAUDE.md.
_explain: """
	## CoffeeCtx

	CoffeeCtx is a knowledge graph MCP server. It holds aggregated information about the project.

	When you are unsure about architecture, a decision that was made, or a symbol that was
	created: first try to discover it via coffeectx MCP tools; if there is no relevant information,
	read the codebase; if there is still nothing, ask the user. Never invent anything.
	"""

// _mcpEntry registers the coffeectx MCP server under the agent. The server bin is
// `coffeectx-mcp` (from @coffeectx/server); a venv install puts it on PATH via
// node_modules/.bin.
_mcpEntry: {name: "coffeectx", command: "coffeectx-mcp"}

// #Job is a coffeectx job: defined exactly like agent.#Skill (name, markdown
// body, optional files). Jobs are registered via #RegisterJob into the shared
// context's coffeectx section and installed into ~/.coffeecode/jobs.
#Job: {
	name:        string
	description: string | *""
	body:        string
	files: {[string]: string} | *{}
}

// #RegisterJob is a self-registering target: place `coffeectx.#RegisterJob &
// {job: {...}}` in a chart's targets list, the same way skills are registered.
// It contributes into the context's coffeectx.jobs section (which #Render
// deep-merges generically).
#RegisterJob: agent.#Target & {
	job: #Job
	register: coffeectx: jobs: (job.name): job
}

// _agentRegister: the agent-driven registration shared by #Install and #Setup —
// always append the explanation, and (when confirmed) register the MCP for
// non-pi agents.
_agentRegister: {
	confirm: bool
	ctx:     agent.#Context
	out: {
		// Nested ifs (not `&&`): a non-concrete `confirm` makes a bare `if`
		// incomplete (tolerated pre-resolution), whereas `&&` hard-errors.
		if confirm {
			if ctx.agent != "pi" {
				mcps: coffeectx: _mcpEntry
			}
		}
		agentMd: [_explain]
	}
}

// #Confirm is the install-confirmation prompt. The resolver only discovers
// @input fields on top-level chart fields (it skips the `states` output), so a
// chart surfaces it as a top-level field — `confirm: coffeectx.#Confirm` — and
// passes the value into #Install/#Setup.
#Confirm: bool @input("Install coffeectx for this agent? (true/false)")

// #SetupInput bundles the prompted, non-per-project configuration for #Setup. A
// chart exposes it as a top-level field so the resolver can prompt it, then
// hands it to #Setup. autolaunch is only meaningful on a global install; on a
// venv install it unifies to a concrete false and is never prompted.
#SetupInput: {
	confirm: bool @input("Install coffeectx for this agent? (true/false)", order=1)

	// Auth follows AuthSettings (retrival-mcp packages/core/src/auth.ts): one
	// common credential shared across every block, a separate model per block.
	// authType is prompted first; the credential fields below only apply in
	// apiKey mode — in openai-oauth mode pi.dev holds the credentials, so they're
	// forced empty and never prompted.
	authType: "apiKey" | "openai-oauth" @input("Auth type (apiKey/openai-oauth)", order=10)
	url:      string @input("API base URL (OpenAI-compatible endpoint)", order=11)
	apiKey:   string @input("API key", order=12)
	if authType == "openai-oauth" {
		url:    ""
		apiKey: ""
	}

	// One model per consumer (embeddings, job agents, UI agent).
	embeddingsModel: string @input("Embeddings model", order=20)
	indexerModel:    string @input("Indexer (job agent) model", order=21)
	uiModel:         string @input("UI agent model", order=22)

	autolaunch: bool @input("Auto-launch the coffeectx daemon on login? (true/false)", order=30)
	if context.engine != "global" {
		autolaunch: false
	}
}

// #Install — install coffeectx for the active agent. `confirm` is supplied by the
// chart (see #Confirm); for pi it installs the pi.dev extension, otherwise it
// registers the MCP server.
#Install: agent.#Target & {
	ctx: agent.#Context

	version: string | *"latest"
	confirm: bool
	_local:  context.engine == "local"
	// _home is the base that holds .coffeecode (and .pi): "~" (i.e. $HOME) for a
	// global install, the venv root for a local one. coffeectx itself looks for
	// .coffeecode under $COFFEECODE_HOME (default $HOME); _envStates points that
	// at the venv on local installs.
	_home: context.root

	register: (_agentRegister & {"confirm": confirm, "ctx": ctx}).out

	// Server binary (coffeectx-mcp) for every agent.
	_serverStates: [
		st.#NpmState & {
			name:    "coffeectx-server"
			package: "@coffeectx/server"
			version: version
			if _local {prefix: context.root}
		},
	]
	// The pi.dev extension, only for the pi agent (and only when confirmed).
	_piStates: [
		if confirm if ctx.agent == "pi" {
			st.#NpmState & {
				name:    "coffeectx-pi-plugin"
				package: "@coffeectx/pi-plugin"
				version: version
				if _local {prefix: context.root}
			}
		},
		if confirm if ctx.agent == "pi" {
			st.#FileState & {
				name:    "coffeectx-pi-ext"
				path:    "\(_home)/.pi/agent/extensions/coffeectx.ts"
				content: "export { default } from '@coffeectx/pi-plugin';\n"
			}
		},
	]
	// Install every context-registered skill into the coffeecode skill dir, the
	// same way agents install them into their own dir.
	_skillStates: [
		for sname, sk in ctx.skills {
			st.#FileState & {
				name:    "coffeecode-skill-\(sname)"
				path:    "\(_home)/.coffeecode/skills/\(sname)/SKILL.md"
				content: sk.body
			}
		},
	] + [
		for sname, sk in ctx.skills for fpath, fcontent in sk.files {
			st.#FileState & {
				name:    "coffeecode-skill-\(sname)-file"
				path:    "\(_home)/.coffeecode/skills/\(sname)/\(fpath)"
				content: fcontent
			}
		},
	]
	// On a local install, point coffeectx at the venv so it reads/writes the
	// venv's .coffeecode rather than $HOME's.
	_envStates: [
		if _local {
			st.#EnvState & {
				name:   "COFFEECODE_HOME"
				value:  context.root
				target: "\(context.root)/env.sh"
			}
		},
	]

	states: _serverStates + _piStates + _skillStates + _envStates
}

// #Project describes one coffeectx project. `name` is supplied by the chart;
// every other field carries @input and is prompted per project. `language`
// selects the LSP server from the lsp catalog; `skills`/`jobs` are
// comma-separated enable lists.
#Project: {
	name:     string
	repoPath: string @input("Repo path", order=1)
	language: string @input("Language for the LSP job (empty for none)", order=2)
	skills:   string @input("Skills to enable, comma-separated (empty for none)", order=3)
	jobs:     string @input("Jobs to enable, comma-separated (empty for none)", order=4)
}

// #Setup — full declarative coffeectx configuration for a list of projects.
#Setup: agent.#Target & {
	// Constrain the context's coffeectx section so registered jobs are typed and
	// default to empty when none are registered.
	ctx: agent.#Context & {coffeectx: jobs: {[string]: #Job} | *{}}

	version: string | *"latest"

	// Prompted config is supplied by the chart via a top-level field (see
	// #SetupInput); destructure it into the locals the body uses.
	input:           #SetupInput
	confirm:         input.confirm
	embeddingsModel: input.embeddingsModel
	indexerModel:    input.indexerModel
	uiModel:         input.uiModel
	autolaunch:      input.autolaunch

	// _authCommon is the shared credential reused by every auth block; each block
	// adds its own model. In openai-oauth mode only authType is carried (pi.dev
	// resolves the rest). Reference input.* directly to avoid a field shadowing
	// its own name (authType: authType would self-cycle).
	_authCommon: {
		authType: input.authType
		if input.authType == "apiKey" {
			url:    input.url
			apiKey: input.apiKey
		}
	}

	// Untyped so `& {projects: chartProjects}` doesn't create a separate
	// unification node that FillPath (applied to the chart field) can't reach.
	// The chart supplies #Project-typed elements.
	projects: [...]
	_local: context.engine == "local"
	// _home is the base that holds .coffeecode (and .pi): "~" (i.e. $HOME) for a
	// global install, the venv root for a local one. See COFFEECODE_HOME below.
	_home: context.root

	// Project a plain-struct list first. A dynamic-key map comprehension
	// (`(p.name): …`) directly over the #Project-typed list yields an
	// incomplete value; projecting to plain structs via a list comprehension
	// first sidesteps that CUE evaluation quirk.
	_plain: [for p in projects {{
		name:     p.name
		repoPath: p.repoPath
		language: p.language
		skills:   p.skills
		jobs:     p.jobs
	}}]

	// <_home>/.coffeecode/config.yaml mirrored from the CoffeectxConfig schema
	// (retrival-mcp packages/core/src/config.ts). Auth is an AuthSettings block
	// per consumer: core.embed.auth (embeddings), agent.auth (UI), and every
	// enabled job's parameters.auth (job agents).
	_config: {
		projects: {
			for p in _plain {
				(p.name): {
					db:       "\(_home)/.coffeecode/db/\(p.name).db"
					repoPath: p.repoPath
					enabled:  true
					core: embed: auth: _authCommon & {model: embeddingsModel}
					agent: auth: _authCommon & {model:       uiModel}
					mcp: tools: {search: true, exact: true, regex: true, raw_query: true, load_node: true, insert: false}
					if p.language != "" {
						jobs: lsp: {enabled: true, parameters: {lspCommand: lsplib.catalog[p.language].command}}
					}
					if p.skills != "" {
						skills: jobs: include: strings.Split(p.skills, ",")
					}
					if p.jobs != "" {
						for jn in strings.Split(p.jobs, ",") {
							jobs: (jn): {enabled: true, parameters: {auth: _authCommon & {model: indexerModel}}}
						}
					}
				}
			}
		}
		types: userDir: "\(_home)/.coffeecode/types"
	}

	register: (_agentRegister & {"confirm": confirm, "ctx": ctx}).out

	// Server binary, for every agent.
	_serverStates: [
		st.#NpmState & {
			name:    "coffeectx-server"
			package: "@coffeectx/server"
			version: version
			if _local {prefix: context.root}
		},
	]
	// The pi.dev extension, only for the pi agent (and only when confirmed).
	_piStates: [
		if confirm if ctx.agent == "pi" {
			st.#NpmState & {
				name:    "coffeectx-pi-plugin"
				package: "@coffeectx/pi-plugin"
				version: version
				if _local {prefix: context.root}
			}
		},
		if confirm if ctx.agent == "pi" {
			st.#FileState & {
				name:    "coffeectx-pi-ext"
				path:    "\(_home)/.pi/agent/extensions/coffeectx.ts"
				content: "export { default } from '@coffeectx/pi-plugin';\n"
			}
		},
	]
	// Install every context-registered skill into the coffeecode skill dir.
	_skillStates: [
		for sname, sk in ctx.skills {
			st.#FileState & {
				name:    "coffeecode-skill-\(sname)"
				path:    "\(_home)/.coffeecode/skills/\(sname)/SKILL.md"
				content: sk.body
			}
		},
	] + [
		for sname, sk in ctx.skills for fpath, fcontent in sk.files {
			st.#FileState & {
				name:    "coffeecode-skill-\(sname)-file"
				path:    "\(_home)/.coffeecode/skills/\(sname)/\(fpath)"
				content: fcontent
			}
		},
	]
	// Install every registered coffeectx job into the coffeecode job dir, the
	// same way as skills.
	_jobStates: [
		for jname, j in ctx.coffeectx.jobs {
			st.#FileState & {
				name:    "coffeecode-job-\(jname)"
				path:    "\(_home)/.coffeecode/jobs/\(jname)/JOB.md"
				content: j.body
			}
		},
	] + [
		for jname, j in ctx.coffeectx.jobs for fpath, fcontent in j.files {
			st.#FileState & {
				name:    "coffeecode-job-\(jname)-file"
				path:    "\(_home)/.coffeecode/jobs/\(jname)/\(fpath)"
				content: fcontent
			}
		},
	]

	_configStates: [
		st.#FileState & {
			name:   "coffeecode-config"
			path:   "\(_home)/.coffeecode/config.yaml"
			format: "yaml"
			data:   _config
		},
	]
	// On a local install, point coffeectx at the venv so it reads/writes the
	// venv's .coffeecode rather than $HOME's.
	_envStates: [
		if _local {
			st.#EnvState & {
				name:   "COFFEECODE_HOME"
				value:  context.root
				target: "\(context.root)/env.sh"
			}
		},
	]
	// In openai-oauth mode the credentials live in pi.dev's auth store, populated
	// by an interactive login. Run it (the server binary providing `coffeectx` is
	// installed by _serverStates above).
	_loginStates: [
		if input.authType == "openai-oauth" {
			st.#ShellState & {
				name: "coffeectx-oauth-login"
				run:  "coffeectx login openai-oauth"
			}
		},
	]

	// Auto-launch: write a launchd plist (darwin) or systemd user unit (linux)
	// that runs `coffeectx daemonize`, then load/enable it.
	_plistPath: "~/Library/LaunchAgents/dev.coffeecode.coffeectx.plist"
	_plist: """
		<?xml version="1.0" encoding="UTF-8"?>
		<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
		<plist version="1.0">
		<dict>
		  <key>Label</key><string>dev.coffeecode.coffeectx</string>
		  <key>ProgramArguments</key>
		  <array><string>coffeectx</string><string>daemonize</string></array>
		  <key>RunAtLoad</key><true/>
		  <key>KeepAlive</key><true/>
		</dict>
		</plist>

		"""
	_unitPath: "~/.config/systemd/user/coffeectx.service"
	_unit: """
		[Unit]
		Description=CoffeeCtx daemon

		[Service]
		ExecStart=coffeectx daemonize
		Restart=on-failure

		[Install]
		WantedBy=default.target

		"""
	_autolaunchStates: [
		if autolaunch if context.os == "darwin" {
			st.#FileState & {
				name:    "coffeectx-launchd"
				path:    _plistPath
				content: _plist
			}
		},
		if autolaunch if context.os == "darwin" {
			st.#ShellState & {
				name:   "coffeectx-launchd-load"
				run:    "launchctl load -w \(_plistPath)"
				unless: "launchctl list | grep -q dev.coffeecode.coffeectx"
			}
		},
		if autolaunch if context.os == "linux" {
			st.#FileState & {
				name:    "coffeectx-systemd"
				path:    _unitPath
				content: _unit
			}
		},
		if autolaunch if context.os == "linux" {
			st.#ShellState & {
				name:   "coffeectx-systemd-enable"
				run:    "systemctl --user enable --now coffeectx.service"
				unless: "systemctl --user is-enabled coffeectx.service >/dev/null 2>&1"
			}
		},
	]

	states: _configStates + _serverStates + _piStates + _skillStates + _jobStates + _envStates + _loginStates + _autolaunchStates
}
