// Package lsp is a standalone, language-keyed catalog of language servers. It
// exports `catalog` (language -> {command, install}) and the #LSP target, which
// takes only a language, installs the server, and exposes the resolved
// `command`. It does not depend on coffeectx; coffeectx imports `catalog` to
// resolve a project's language into the command it writes to config.yaml.
package lsp

import (
	"strings"
	"coffeeenv.dev/lib/agent"
	st "coffeeenv.dev/lib/states"
)

// catalog maps a language to how its LSP server runs (`command`) and how to
// install it (`install`). Exported (no leading underscore) so other packages
// can read it. Add languages here as the single source of truth.
catalog: {
	typescript: {command: "typescript-language-server --stdio", install: "npm i -g typescript-language-server typescript"}
	go: {command:         "gopls", install:                     "go install golang.org/x/tools/gopls@latest"}
	python: {command:     "pylsp", install:                     "pip install python-lsp-server"}
	rust: {command:       "rust-analyzer", install:             "rustup component add rust-analyzer"}
}

// #LSP installs the language server for `language` and exposes the resolved
// `command`. Input is only the language; everything else comes from `catalog`.
#LSP: agent.#Target & {
	language: string @input("Language for the LSP server")

	_entry:  catalog[language]
	command: _entry.command

	states: [
		st.#ShellState & {
			name:   "lsp-install-\(language)"
			run:    _entry.install
			unless: "command -v \(strings.Fields(_entry.command)[0]) >/dev/null 2>&1"
		},
	]
}
