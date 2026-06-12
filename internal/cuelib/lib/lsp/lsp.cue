// Package lsp is a standalone registry of language servers. Servers are
// registered under the shared `lsp.available` namespace (root-module top level)
// the same way skills are registered under `agent.skills`. Each #Lsp holds how
// to run the server (`command`) and a #State that installs it (`installState`),
// but does NOT push that state into the global `states` map — a consumer lifts
// it: #InstallLsp installs a selected set of languages, and coffeectx delegates
// to it for the languages its projects use. It does not depend on coffeectx.
package lsp

import (
	"coffeeenv.dev/lib/context"
	st "coffeeenv.dev/lib/states"
)

// #Lsp is one language server. `command` is how to run it; `installState` is the
// state that installs it — ANY #State (npm today, other package managers later,
// or a guarded shell command), not necessarily a shell. Stored here, lifted into
// the global states map by #InstallLsp / coffeectx.
#Lsp: {
	command:      string
	installState: st.#State
}

// #LspNS is the shared `lsp` namespace. `available` maps a language to its #Lsp,
// the same way agent.skills maps a name to a #Skill.
#LspNS: {
	available: {[string]: #Lsp}
}

// _builtin is the in-built catalog registered by #Setup. typescript installs via
// npm (native idempotency); the others install via a guarded shell command.
_builtin: {
	typescript: {
		command: "typescript-language-server --stdio"
		installState: st.#NpmState & {
			package: "typescript-language-server"
			// Local engine: install into the venv (bins land in node_modules/.bin).
			if context.engine == "local" {prefix: context.root}
		}
	}
	go: {
		command:      "gopls"
		installState: st.#ShellState & {run: "go install golang.org/x/tools/gopls@latest", unless: "command -v gopls >/dev/null 2>&1"}
	}
	python: {
		command:      "pylsp"
		installState: st.#ShellState & {run: "pip install python-lsp-server", unless: "command -v pylsp >/dev/null 2>&1"}
	}
	rust: {
		command:      "rust-analyzer"
		installState: st.#ShellState & {run: "rustup component add rust-analyzer", unless: "command -v rust-analyzer >/dev/null 2>&1"}
	}
}

// #Setup registers the in-built language servers into lsp.available. A chart (or
// coffeectx) embeds it; the user can add more via `lsp: available: <lang>: {…}`.
#Setup: {
	lsp: #LspNS
	lsp: available: _builtin
}

// #InstallLsp lifts the install state of each selected language into the global
// states map (replaces the old #LSP). Languages must be registered in
// lsp.available. Embed it: `lsp.#InstallLsp & {languages: ["go"]}`.
#InstallLsp: {
	lsp: #LspNS
	languages: [...string]
	states: {[string]: st.#State}
	states: {
		for lang in languages {
			"lsp-install-\(lang)": lsp.available[lang].installState
		}
	}
}
