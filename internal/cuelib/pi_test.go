package cuelib

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPiLocal: the pi target installs @earendil-works/pi-coding-agent, isolates
// the agent dir via PI_CODING_AGENT_DIR in venv mode, renders extensions, and —
// since pi has no native MCP — writes mcp.json and installs the pi-mcp-adapter
// package via `pi install`, guarded by `pi list`.
func TestPiLocal(t *testing.T) {
	root := t.TempDir()
	raws, err := EvalStates(exampleDir("pi-basic"), Opts{Engine: "local", Root: root})
	if err != nil {
		t.Fatalf("EvalStates: %v", err)
	}
	m := byName(raws)
	agentDir := filepath.Join(root, ".pi", "agent")

	pi, ok := m["pi"]
	if !ok {
		t.Fatalf("missing pi install state; got %v", names(raws))
	}
	if got, _ := pi.Params["package"].(string); got != "@earendil-works/pi-coding-agent" {
		t.Errorf("pi package = %q", got)
	}
	if got, _ := pi.Params["prefix"].(string); got != root {
		t.Errorf("pi prefix = %q, want venv root %q", got, root)
	}

	env, ok := m["PI_CODING_AGENT_DIR"]
	if !ok {
		t.Fatalf("missing PI_CODING_AGENT_DIR env state; got %v", names(raws))
	}
	if got, _ := env.Params["value"].(string); got != agentDir {
		t.Errorf("PI_CODING_AGENT_DIR = %q, want %q", got, agentDir)
	}

	ext, ok := m["pi-extension-greet"]
	if !ok {
		t.Fatalf("missing pi-extension-greet; got %v", names(raws))
	}
	if got, _ := ext.Params["path"].(string); got != filepath.Join(agentDir, "extensions", "greet.ts") {
		t.Errorf("extension path = %q", got)
	}

	skill, ok := m["pi-skill-hello"]
	if !ok {
		t.Fatalf("missing pi-skill-hello; got %v", names(raws))
	}
	if got, _ := skill.Params["path"].(string); got != filepath.Join(agentDir, "skills", "hello", "SKILL.md") {
		t.Errorf("skill path = %q", got)
	}

	mcp, ok := m["pi-mcp"]
	if !ok {
		t.Fatalf("missing pi-mcp (mcp.json); got %v", names(raws))
	}
	if got, _ := mcp.Params["path"].(string); got != filepath.Join(agentDir, "mcp.json") {
		t.Errorf("mcp.json path = %q", got)
	}

	adapter, ok := m["pi-package-pi-mcp-adapter"]
	if !ok {
		t.Fatalf("missing pi-package-pi-mcp-adapter; got %v", names(raws))
	}
	if got, _ := adapter.Params["run"].(string); got != "pi install npm:pi-mcp-adapter" {
		t.Errorf("adapter run = %q", got)
	}
	if got, _ := adapter.Params["unless"].(string); !strings.Contains(got, "pi list") {
		t.Errorf("adapter unless should use pi list; got %q", got)
	}
}

// TestPiGlobal: global mode targets ~/.pi/agent and sets no PI_CODING_AGENT_DIR.
func TestPiGlobal(t *testing.T) {
	raws, err := EvalStates(exampleDir("pi-basic"), Opts{Engine: "global", Root: "~"})
	if err != nil {
		t.Fatalf("EvalStates: %v", err)
	}
	m := byName(raws)
	if _, ok := m["PI_CODING_AGENT_DIR"]; ok {
		t.Errorf("global install should not set PI_CODING_AGENT_DIR; got %v", names(raws))
	}
	ext, ok := m["pi-extension-greet"]
	if !ok {
		t.Fatalf("missing pi-extension-greet; got %v", names(raws))
	}
	if got, _ := ext.Params["path"].(string); got != "~/.pi/agent/extensions/greet.ts" {
		t.Errorf("extension path = %q, want ~/.pi/agent/...", got)
	}
}

func TestPiHostedMcp(t *testing.T) {
	dir := t.TempDir()
	chart := `package env

import "coffeeenv.dev/lib/agent/pi"

#Main: {
	pi.#Main
	agent: mcps: slack: {
		transport: "streamable-http"
		url:       "https://mcp.slack.com/mcp"
	}
}

#Main
`
	if err := os.WriteFile(filepath.Join(dir, "env.cue"), []byte(chart), 0o644); err != nil {
		t.Fatal(err)
	}
	raws, err := EvalStates(dir, Opts{Engine: "global", Root: "~"})
	if err != nil {
		t.Fatalf("EvalStates: %v", err)
	}
	m := byName(raws)
	mcp, ok := m["pi-mcp"]
	if !ok {
		t.Fatalf("missing pi-mcp; got %v", names(raws))
	}
	data, _ := mcp.Params["data"].(map[string]any)
	servers, _ := data["mcpServers"].(map[string]any)
	slack, _ := servers["slack"].(map[string]any)
	if slack == nil {
		t.Fatalf("missing slack MCP server; data=%#v", data)
	}
	if got, _ := slack["transport"].(string); got != "streamable-http" {
		t.Errorf("transport = %q, want streamable-http", got)
	}
	if got, _ := slack["url"].(string); got != "https://mcp.slack.com/mcp" {
		t.Errorf("url = %q", got)
	}
}

// TestPiNoMcp: a pi chart with no MCP servers gets neither mcp.json nor the
// adapter package.
func TestPiNoMcp(t *testing.T) {
	dir := t.TempDir()
	chart := `package env

import "coffeeenv.dev/lib/agent/pi"

#Main: {
	pi.#Main
	agent: skills: hi: {body: "x"}
}

#Main
`
	if err := os.WriteFile(filepath.Join(dir, "env.cue"), []byte(chart), 0o644); err != nil {
		t.Fatal(err)
	}
	raws, err := EvalStates(dir, Opts{Engine: "global", Root: "~"})
	if err != nil {
		t.Fatalf("EvalStates: %v", err)
	}
	m := byName(raws)
	if _, ok := m["pi-mcp"]; ok {
		t.Errorf("no-mcp chart should not write mcp.json; got %v", names(raws))
	}
	if _, ok := m["pi-package-pi-mcp-adapter"]; ok {
		t.Errorf("no-mcp chart should not install the adapter; got %v", names(raws))
	}
}
