// Package states defines the fixed-parameter basic state schemas that the Go
// execution layer understands. High-level helpers (claude, codex, coffeectx)
// expand into a map of these, keyed by a stable name — the map key IS the
// state's name (the Go layer reads it from the key).
//
// `order` gives the Go layer a stable apply order independent of map iteration:
// states are flattened sorted by (order, key). Defaults group by kind —
// installs (25) before files (50) before env (60) before shell (75) — and ties
// break by key.
package states

// #NpmState installs a package. With prefix empty it installs globally (-g);
// with prefix set it installs into that directory (bins land in <prefix>/bin).
#NpmState: {
	type:     "npm"
	order:    int | *25
	package:  string
	version:  string | *"latest"
	prefix?:  string
	bin?: [...string]
}

// #PnpmState installs a package via pnpm. prefix behaves as for #NpmState.
#PnpmState: {
	type:    "pnpm"
	order:   int | *25
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
	order:    int | *50
	path:     string
	mode:     int | *0o644
	content?: string
	data?: {...}
	format?: "json" | "toml" | "yaml"
}

// #CopyState recursively copies a filesystem tree from src into dst at apply
// time. Used for path-sourced skills/jobs (`files: "<path>"`). A relative src is
// resolved against the chart directory by the Go layer.
#CopyState: {
	type:  "copy"
	order: int | *50
	src:   string
	dst:   string
}

// #EnvState manages one environment variable in a managed export file. target
// is the activate file path; empty means the global ~/.config/coffeeenv/activate.sh.
// expand: when true the value is double-quoted so shell references like $PATH
// expand when sourced (e.g. PATH prepends: "<dir>:$PATH").
#EnvState: {
	type:    "env"
	order:   int | *60
	value:   string
	target?: string
	expand?: bool
}

// #ShellState runs a command, optionally guarded for idempotency.
#ShellState: {
	type:     "shell"
	order:    int | *75
	run:      string
	creates?: string
	unless?:  string
}

// #State is any basic state; used to type the top-level states map.
#State: #NpmState | #PnpmState | #FileState | #CopyState | #EnvState | #ShellState
