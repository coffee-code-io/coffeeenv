// Package coffeectx installs and configures the coffeectx knowledge-graph
// toolchain for the active agent, as a mixin over shared global state. A chart
// embeds #Mcp (minimal: server install + agent integration + the explanation
// paragraph) or #Setup (full: also generates ~/.coffeecode/config.yaml, installs
// context skills and coffeectx jobs, runs the oauth login, and registers the
// daemon for auto-launch). Both read the `coffeectx` namespace (config + jobs)
// and the `agent` namespace, and FEED the agent target by writing
// `agent.mcps.coffeectx` / `agent.md.coffeectx` — which the agent target then
// renders into its own files.
//
// The agent integration is driven by the active agent (agent.name, set by the
// agent target): for "pi" we install the pi.dev extension, otherwise we register
// the MCP server. Either way it is gated behind a confirmation prompt.
package coffeectx

import (
	"strings"
	"coffeeenv.dev/lib/context"
	ag "coffeeenv.dev/lib/agent"
	// Aliased: the generated config has a `jobs.lsp` field that would otherwise
	// shadow this package name where the command is resolved.
	lsplib "coffeeenv.dev/lib/lsp"
	st "coffeeenv.dev/lib/states"
)

// _explain is the CoffeeCtx paragraph fed into the agent's AGENTS.md/CLAUDE.md.
_explain: """
	## CoffeeCtx

	CoffeeCtx is a knowledge graph MCP server. It holds aggregated information about the project.

	When you are unsure about architecture, a decision that was made, or a symbol that was
	created: first try to discover it via coffeectx MCP tools; if there is no relevant information,
	read the codebase; if there is still nothing, ask the user. Never invent anything.
	"""

// #Job is a coffeectx job: content from an inline `body` (JOB.md) or `files` (a
// path, copied in, or an inline relpath->content map), mirroring agent.#Skill.
// A chart registers a job by writing data: `coffeectx: jobs: reindex: {...}`.
#Job: {
	description?: string
	body:         string | *""
	files:        string | {[string]: string} | *{}
}

// #Project describes one coffeectx project. The map key is the project name;
// every field carries @input and is prompted per project. `language` selects the
// LSP server from the lsp catalog; `skills`/`jobs` are comma-separated enable
// lists.
#Project: {
	repoPath: string @input("Repo path", order=1)
	language: string @input("Language for the LSP job (empty for none)", order=2)
	skills:   string @input("Skills to enable, comma-separated (empty for none)", order=3)
	jobs:     string @input("Jobs to enable, comma-separated (empty for none)", order=4)
}

// #McpNS is the minimal `coffeectx` namespace used by #Mcp: just the install
// confirmation and any registered jobs. The full #CtxNS embeds it, so a chart
// that only installs the MCP integration is prompted for `confirm` alone, while
// #Setup adds the auth/model/project configuration below.
#McpNS: {
	confirm: bool @input("Install coffeectx for this agent? (true/false)", order=1)
	jobs: {[string]: #Job}
}

// #CtxNS is the full `coffeectx` namespace: the minimal #McpNS plus the prompted
// setup configuration used by #Setup. Auth follows AuthSettings (retrival-mcp
// packages/core/src/auth.ts): one common credential, a separate model per block.
// authType is prompted first; in openai-oauth mode pi.dev holds the credentials
// so url/apiKey are forced empty and never prompted. autolaunch is only
// meaningful on a global install.
#CtxNS: {
	#McpNS

	authType: "apiKey" | "openai-oauth" @input("Auth type (apiKey/openai-oauth)", order=2)
	url:      string @input("API base URL (OpenAI-compatible endpoint)", order=3)
	apiKey:   string @input("API key", order=4)
	if authType == "openai-oauth" {
		url:    ""
		apiKey: ""
	}

	embeddingsModel: string @input("Embeddings model", order=5)
	indexerModel:    string @input("Indexer (job agent) model", order=6)
	uiModel:         string @input("UI agent model", order=7)

	if context.engine == "global" {
		autolaunch: bool @input("Auto-launch the coffeectx daemon on login? (true/false)", order=8)
	}
	if context.engine != "global" {
		autolaunch: false
	}

	projects: {[string]: #Project} @inputMap("Project name")
}

// #Mcp installs coffeectx for the active agent and feeds the agent namespace.
// `coffeectx.confirm` gates the integration; for pi it installs the pi.dev
// extension, otherwise it registers the MCP server (by writing
// agent.mcps.coffeectx, which the agent target renders).
#Mcp: {
	coffeectx: #McpNS
	agent: ag.#NS
	states: {[string]: st.#State}

	_local:   context.engine == "local"
	_home:    context.root
	_version: string | *"latest"

	// Feed the agent namespace: the explanation is always added; the MCP server
	// is registered for non-pi agents when confirmed. The confirm-gated entry
	// lives inside the agent.mcps value (the field it contributes to). Chained
	// `if` (not `&&`) because `&&` hard-errors on a non-concrete operand.
	agent: md: coffeectx: _explain
	agent: mcps: {
		if coffeectx.confirm if agent.name != "pi" {
			coffeectx: {command: "coffeectx-mcp"}
		}
	}

	states: {
		"coffeectx-server": st.#NpmState & {
			package: "@coffeectx/server"
			version: _version
			if _local {prefix: context.root}
		}
		if coffeectx.confirm if agent.name == "pi" {
			"coffeectx-pi-plugin": st.#NpmState & {
				package: "@coffeectx/pi-plugin"
				version: _version
				if _local {prefix: context.root}
			}
		}
		if coffeectx.confirm if agent.name == "pi" {
			"coffeectx-pi-ext": st.#FileState & {
				path:    "\(_home)/.pi/agent/extensions/coffeectx.ts"
				content: "export { default } from '@coffeectx/pi-plugin';\n"
			}
		}
	}
}

// #Setup is the full declarative configuration. It embeds #Mcp (server +
// integration) and adds the config file, skill/job installs, the oauth login,
// and auto-launch. It re-declares `coffeectx`/`agent` as explicit fields so its
// own comprehensions resolve those namespaces after the mixin is embedded.
#Setup: {
	#Mcp
	coffeectx: #CtxNS
	agent: ag.#NS

	_local: context.engine == "local"
	_home:  context.root

	// _authCommon is the shared credential reused by every auth block; each block
	// adds its own model. In openai-oauth mode only authType is carried.
	_authCommon: {
		authType: coffeectx.authType
		if coffeectx.authType == "apiKey" {
			url:    coffeectx.url
			apiKey: coffeectx.apiKey
		}
	}

	// <_home>/.coffeecode/config.yaml mirrored from CoffeectxConfig (retrival-mcp
	// packages/core/src/config.ts). The leading `{}` seeds the projects map so the
	// zero-project case is a concrete `{}` rather than an empty comprehension
	// (which CUE otherwise carries as incomplete once embedded in `data`).
	_config: {
		projects: {
			{}
			for k, p in coffeectx.projects {
				(k): {
					db:       "\(_home)/.coffeecode/db/\(k).db"
					repoPath: p.repoPath
					enabled:  true
					core: embed: auth: _authCommon & {model: coffeectx.embeddingsModel}
					agent: auth: _authCommon & {model:       coffeectx.uiModel}
					mcp: tools: {search: true, exact: true, regex: true, raw_query: true, load_node: true, insert: false}
					if p.language != "" {
						jobs: lsp: {enabled: true, parameters: {lspCommand: lsplib.catalog[p.language].command}}
					}
					if p.skills != "" {
						skills: jobs: include: strings.Split(p.skills, ",")
					}
					if p.jobs != "" {
						for jn in strings.Split(p.jobs, ",") {
							jobs: (jn): {enabled: true, parameters: {auth: _authCommon & {model: coffeectx.indexerModel}}}
						}
					}
				}
			}
		}
		types: userDir: "\(_home)/.coffeecode/types"
	}

	states: {
		"coffeecode-config": st.#FileState & {
			path:   "\(_home)/.coffeecode/config.yaml"
			format: "yaml"
			data:   _config
		}

		// Install every context-registered skill into the coffeecode skill dir.
		for sname, sk in agent.skills if sk.body != "" {
			"coffeecode-skill-\(sname)": st.#FileState & {
				path:    "\(_home)/.coffeecode/skills/\(sname)/SKILL.md"
				content: sk.body
			}
		}
		for sname, sk in agent.skills if (sk.files & string) != _|_ {
			"coffeecode-skill-\(sname)-files": st.#CopyState & {
				src: sk.files
				dst: "\(_home)/.coffeecode/skills/\(sname)"
			}
		}
		for sname, sk in agent.skills if (sk.files & {[string]: string}) != _|_ for fpath, fcontent in sk.files {
			"coffeecode-skill-\(sname)-\(fpath)": st.#FileState & {
				path:    "\(_home)/.coffeecode/skills/\(sname)/\(fpath)"
				content: fcontent
			}
		}

		// Install every registered coffeectx job into the coffeecode job dir.
		for jname, j in coffeectx.jobs if j.body != "" {
			"coffeecode-job-\(jname)": st.#FileState & {
				path:    "\(_home)/.coffeecode/jobs/\(jname)/JOB.md"
				content: j.body
			}
		}
		for jname, j in coffeectx.jobs if (j.files & string) != _|_ {
			"coffeecode-job-\(jname)-files": st.#CopyState & {
				src: j.files
				dst: "\(_home)/.coffeecode/jobs/\(jname)"
			}
		}
		for jname, j in coffeectx.jobs if (j.files & {[string]: string}) != _|_ for fpath, fcontent in j.files {
			"coffeecode-job-\(jname)-\(fpath)": st.#FileState & {
				path:    "\(_home)/.coffeecode/jobs/\(jname)/\(fpath)"
				content: fcontent
			}
		}

		// On a local install, point coffeectx at the venv ($COFFEECODE_HOME).
		if _local {
			"COFFEECODE_HOME": st.#EnvState & {
				value:  context.root
				target: "\(context.root)/env.sh"
			}
		}

		// openai-oauth mode: credentials live in pi.dev's auth store, populated by
		// an interactive login.
		if coffeectx.authType == "openai-oauth" {
			"coffeectx-oauth-login": st.#ShellState & {
				run: "coffeectx login openai-oauth"
			}
		}

		// Auto-launch: launchd plist (darwin) / systemd user unit (linux).
		if coffeectx.autolaunch if context.os == "darwin" {
			"coffeectx-launchd": st.#FileState & {
				path:    _plistPath
				content: _plist
			}
		}
		if coffeectx.autolaunch if context.os == "darwin" {
			"coffeectx-launchd-load": st.#ShellState & {
				run:    "launchctl load -w \(_plistPath)"
				unless: "launchctl list | grep -q dev.coffeecode.coffeectx"
			}
		}
		if coffeectx.autolaunch if context.os == "linux" {
			"coffeectx-systemd": st.#FileState & {
				path:    _unitPath
				content: _unit
			}
		}
		if coffeectx.autolaunch if context.os == "linux" {
			"coffeectx-systemd-enable": st.#ShellState & {
				run:    "systemctl --user enable --now coffeectx.service"
				unless: "systemctl --user is-enabled coffeectx.service >/dev/null 2>&1"
			}
		}
	}
}

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
