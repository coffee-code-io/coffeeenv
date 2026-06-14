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
	"time"
	"coffeeenv.dev/lib/context"
	core "coffeeenv.dev/lib/core"
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
// LSP server from the lsp.available registry; `skills`/`jobs` are comma-separated
// enable lists.
#Project: {
	repoPath: string @input("Repo path", order=1)
	language: string @input("Language for the LSP job (empty for none)", order=2)
	// Multi-select from the registered skills/jobs (agent.skills / coffeectx.jobs).
	skills: [...string] @multichoice("Enable skills", from=agent.skills, order=3)
	jobs: [...string] @multichoice("Enable jobs", from=coffeectx.jobs, order=4)
}

// #McpNS is the minimal `coffeectx` namespace used by #Mcp: just any registered
// jobs. Embedding #Mcp / #Setup is itself the opt-in — there is no confirmation
// prompt in the library (a chart that wants one adds it in its own env.cue). The
// full #CtxNS embeds this, so #Setup adds the auth/model/project config below.
#McpNS: {
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

	authType: "apiKey" | "openai-oauth" @choose("Auth type", order=1)

	// Main credential (UI agent + jobs). AuthSettings carries provider XOR url:
	// provider wins, so url is only prompted when provider is left empty. In
	// oauth mode pi.dev holds the credential, so all three are forced empty.
	provider: string @input("Provider alias openai/anthropic/openrouter (empty for custom url)", order=2)
	url:      string @input("Custom API base URL", order=3)
	apiKey:   string @input("API key", order=4)
	if provider != "" {
		url: ""
	}
	if authType == "openai-oauth" {
		provider: ""
		url:      ""
		apiKey:   ""
	}

	// Embeddings credential — always apiKey, since openai-oauth can't embed. In
	// apiKey mode it reuses the main credential (pinned, never prompted); in oauth
	// mode it is prompted separately.
	embedProvider: string @input("Embeddings provider alias (empty for custom url)", order=5)
	embedUrl:      string @input("Embeddings custom API base URL", order=6)
	embedApiKey:   string @input("Embeddings API key", order=7)
	if embedProvider != "" {
		embedUrl: ""
	}
	if authType == "apiKey" {
		embedProvider: provider
		embedUrl:      url
		embedApiKey:   apiKey
	}

	embeddingsModel: string @input("Embeddings model", order=8)
	indexerModel:    string @input("Indexer (job agent) model", order=9)
	uiModel:         string @input("UI agent model", order=10)

	if context.engine == "global" {
		autolaunch: bool @input("Auto-launch the coffeectx daemon on login? (true/false)", order=11)
	}
	if context.engine != "global" {
		autolaunch: false
	}

	// Projects are entered first (order=12); the active project is then chosen
	// from the entered keys (order=13). CoffeectxConfig.active; empty = unset
	// (e.g. when no projects were entered).
	projects: {[string]: #Project} @inputMap("Project name", order=12)
	active:   string @choose("Active project", from=coffeectx.projects, order=13)
}

// #Mcp installs coffeectx for the active agent and feeds the agent namespace.
// The indexer (the daemon/CLI) is always installed. The integration is driven by
// the active agent (agent.name): for "pi" we install the pi.dev plugin +
// extension; otherwise we install the MCP server package (@coffeectx/server,
// which provides the `coffeectx-mcp` binary) and register agent.mcps.coffeectx,
// which the agent target renders.
#Mcp: {
	coffeectx: #McpNS
	agent: ag.#NS
	states: {[string]: st.#State}

	_local:   context.engine == "local"
	_home:    context.root
	_version: string | *"latest"

	// Feed the agent namespace: the explanation is always added; the MCP server
	// is registered for non-pi agents (the entry lives inside the agent.mcps
	// value, the field it contributes to). agent.name is concrete, so no gating
	// on an unresolved input here.
	agent: md: coffeectx: _explain
	agent: mcps: {
		if agent.name != "pi" {
			coffeectx: {command: "coffeectx-mcp"}
		}
	}

	states: {
		// The indexer (daemon + `coffeectx` CLI) is always installed.
		"coffeectx-indexer": st.#NpmState & {
			package: "@coffeectx/indexer"
			version: _version
			if _local {prefix: context.root}
		}
		// The MCP server package — only for non-pi agents.
		if agent.name != "pi" {
			"coffeectx-server": st.#NpmState & {
				package: "@coffeectx/server"
				version: _version
				if _local {prefix: context.root}
			}
		}
		// The pi.dev plugin + extension — only for the pi agent.
		if agent.name == "pi" {
			"coffeectx-pi-plugin": st.#NpmState & {
				package: "@coffeectx/pi-plugin"
				version: _version
				if _local {prefix: context.root}
			}
		}
		if agent.name == "pi" {
			"coffeectx-pi-ext": st.#FileState & {
				path:    "\(_home)/.pi/agent/extensions/coffeectx.ts"
				content: "export { default } from '@coffeectx/pi-plugin';\n"
			}
		}
	}
}

// #Main is the full declarative configuration (the coffeectx executable target).
// It embeds #Mcp (server + integration) and adds the config file, skill/job
// installs, the oauth login, and auto-launch. It re-declares `coffeectx`/`agent`
// as explicit fields so its own comprehensions resolve those namespaces after
// the mixin is embedded.
#Main: {
	core.#Main
	#Mcp
	coffeectx: #CtxNS
	agent: ag.#NS

	// Require the in-built language servers, and install only the ones the
	// projects use. The user can register more via `lsp: available: <lang>: {…}`.
	lsplib.#Setup
	lsplib.#Main & {languages: _langs}
	lsp: lsplib.#LspNS
	_langs: [for k, p in coffeectx.projects if p.language != "" {p.language}]
	// Alias the registry to a name the config's `jobs.lsp` field can't shadow.
	_lspAvail: lsp.available
	// Alias the active agent name: inside a project struct (which defines its own
	// `agent: auth: …`) a bare `agent` would resolve to that, not the namespace.
	_agentName: agent.name

	_local: context.engine == "local"
	_home:  context.root

	// Two AuthSettings builders (retrival-mcp packages/core/src/auth.ts): provider
	// XOR url, each block adding its own model. _mainAuth drives the UI agent and
	// jobs (openai-oauth carries only authType); _embedAuth is always apiKey since
	// oauth can't embed, reusing the main credential in apiKey mode.
	_mainAuth: {
		authType: coffeectx.authType
		if coffeectx.authType == "apiKey" {
			if coffeectx.provider != "" {provider: coffeectx.provider}
			if coffeectx.provider == "" {url: coffeectx.url}
			apiKey: coffeectx.apiKey
		}
	}
	_embedAuth: {
		authType: "apiKey"
		if coffeectx.embedProvider != "" {provider: coffeectx.embedProvider}
		if coffeectx.embedProvider == "" {url: coffeectx.embedUrl}
		apiKey: coffeectx.embedApiKey
	}

	// <_home>/.coffeecode/config.yaml mirrored from CoffeectxConfig (retrival-mcp
	// packages/core/src/config.ts). The leading `{}` seeds the projects map so the
	// zero-project case is a concrete `{}` rather than an empty comprehension
	// (which CUE otherwise carries as incomplete once embedded in `data`).
	_config: {
		if coffeectx.active != "" {
			active: coffeectx.active
		}
		projects: {
			{}
			for k, p in coffeectx.projects {
				(k): {
					db:       "\(_home)/.coffeecode/db/\(k).db"
					repoPath: p.repoPath
					enabled:  true
					core: embed: auth: _embedAuth & {model: coffeectx.embeddingsModel}
					agent: auth: _mainAuth & {model:        coffeectx.uiModel}
					mcp: tools: {search: true, exact: true, regex: true, raw_query: true, load_node: true, insert: false}

					// In-built jobs. Agent-log import: emit all three agent keys but
					// enable only the active one; only claude carries import params
					// today (codex/pi on-disk log schema isn't wired yet, so they stay
					// off). Logs live in the user's real home, hence the literal "~".
					jobs: claude: {
						enabled: _agentName == "claude"
						if _agentName == "claude" {
							parameters: {
								path:       "~/.claude/projects/\(strings.Replace(p.repoPath, "/", "-", -1))"
								newerThan:  time.Unix(context.nowUnix-86400, 0) // since yesterday
								intervalMs: 30000
							}
						}
						if _agentName != "claude" {parameters: null}
					}
					jobs: codex: {enabled: false, parameters: null}
					jobs: pi: {enabled: false, parameters: null}

					// Always-on framework jobs: plan import, the indexer (carries the
					// indexer-agent credential), and span linking.
					jobs: plans: {enabled: true, parameters: null}
					jobs: indexer: {enabled: true, parameters: {auth: _mainAuth & {model: coffeectx.indexerModel}}}
					jobs: "span-link": {enabled: true}

					if p.language != "" {
						jobs: lsp: {enabled: true, parameters: {lspCommand: _lspAvail[p.language].command}}
					}
					if len(p.skills) > 0 {
						skills: jobs: include: p.skills
					}
					// User-registered jobs enabled via the project's @multichoice.
					for jn in p.jobs {
						jobs: (jn): {enabled: true, parameters: {auth: _mainAuth & {model: coffeectx.indexerModel}}}
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

		// Initialize each project once the config is written (shell states run
		// after files). Guarded by the project db so init runs only once.
		for k, p in coffeectx.projects {
			"coffeectx-init-\(k)": st.#ShellState & {
				run:     "coffeectx init \(k)"
				creates: "\(_home)/.coffeecode/db/\(k).db"
			}
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
