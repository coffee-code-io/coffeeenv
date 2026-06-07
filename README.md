# coffeeenv

A declarative environment manager for AI coding setups, driven by [CUE](https://cuelang.org). Define
an environment as a CUE **chart** — npm/pnpm packages, files, env vars, shell steps, MCP servers,
skills, AGENTS.md — then `plan` and `apply` it to the real machine or into an isolated **venv**.

```
coffeeenv pull <source>            # fetch a chart into ~/.coffeeenv/charts
coffeeenv plan  <chart>            # show what would change
coffeeenv apply <chart>            # converge the machine
coffeeenv venv create <name>       # a local environment
coffeeenv apply --venv <name> <chart>     # install into the venv (engine=local)
coffeeenv apply --materialize <name>      # re-render the venv's chart globally
```

## Install

```
go install github.com/coffee-code-io/coffeeenv@latest
# or via npm (bundled binary):
npm i -g @coffeectx/coffeeenv
```

## How it works

A chart is a `package env` CUE file that imports the bundled library
(`coffeeenv.dev/lib/...`) and produces a flat `states` list. The library exposes a **polymorphic
agent framework**: agent-agnostic features (skills, MCP servers, AGENTS.md parts) register into a
shared Context that an agent target (`claude.#Claude`, `codex.#Codex`) renders into that agent's
native layout. Go injects an engine context (`global` vs a local venv) so the same chart materializes
to the real machine or into a venv directory.

See `examples/` for charts.

## Build

```
go build ./...           # the CLI (self-contained; CUE library is go:embed'd)
go test ./...
node npm/scripts/build-binaries.mjs   # cross-compile all platforms into npm/vendor
```
