// Package states defines the fixed-parameter basic state schemas that the Go
// execution layer understands. High-level helpers (claude, codex) expand into
// lists of these.
package states

// #NpmState installs a package. With prefix empty it installs globally (-g);
// with prefix set it installs into that directory (bins land in <prefix>/bin).
#NpmState: {
	type:     "npm"
	name:     string
	package:  string
	version:  string | *"latest"
	prefix?:  string
	bin?: [...string]
}

// #PnpmState installs a package via pnpm. prefix behaves as for #NpmState.
#PnpmState: {
	type:    "pnpm"
	name:    string
	package: string
	version: string | *"latest"
	prefix?: string
}

// #FileState writes a file with exact content and mode. path may use ~ and
// ${VAR}, expanded by the Go layer. Content is either a literal `content`
// string, or a structured `data` subtree rendered by the Go layer in `format`
// (json/toml/yaml).
#FileState: {
	type:     "file"
	name:     string
	path:     string
	mode:     int | *0o644
	content?: string
	data?: {...}
	format?: "json" | "toml" | "yaml"
}

// #EnvState manages one environment variable in a managed export file. target
// is the activate file path; empty means the global ~/.config/coffeeenv/activate.sh.
// expand: when true the value is double-quoted so shell references like $PATH
// expand when sourced (e.g. PATH prepends: "<dir>:$PATH").
#EnvState: {
	type:    "env"
	name:    string
	value:   string
	target?: string
	expand?: bool
}

// #ShellState runs a command, optionally guarded for idempotency.
#ShellState: {
	type:     "shell"
	name:     string
	run:      string
	creates?: string
	unless?:  string
}

// #State is any basic state; used to type the top-level list.
#State: #NpmState | #PnpmState | #FileState | #EnvState | #ShellState
