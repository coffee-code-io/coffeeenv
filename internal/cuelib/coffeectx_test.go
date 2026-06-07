package cuelib

import (
	"strings"
	"testing"
)

// TestCoffeectxInstall: the no-prompt module installs @coffeectx/server, registers
// the MCP server, and adds the CoffeeCtx paragraph (via the Claude renderer).
func TestCoffeectxInstall(t *testing.T) {
	raws, err := EvalStates(exampleDir("claude-coffeectx"), Opts{Engine: "global", Root: "~"})
	if err != nil {
		t.Fatalf("EvalStates: %v", err)
	}
	m := byName(raws)
	if _, ok := m["coffeectx-server"]; !ok {
		t.Errorf("missing @coffeectx/server npm state; got %v", names(raws))
	}
	md, ok := m["claude-claudemd"]
	if !ok {
		t.Fatalf("missing CLAUDE.md; got %v", names(raws))
	}
	if got, _ := md.Params["content"].(string); !strings.Contains(got, "## CoffeeCtx") {
		t.Errorf("CLAUDE.md missing CoffeeCtx paragraph: %q", got)
	}
	mcp, ok := m["claude-mcp"]
	if !ok {
		t.Fatalf("missing claude-mcp; got %v", names(raws))
	}
	data, _ := mcp.Params["data"].(map[string]any)
	servers, _ := data["mcpServers"].(map[string]any)
	if _, ok := servers["coffeectx"]; !ok {
		t.Errorf("mcpServers should include coffeectx; got %#v", servers)
	}
}

// TestCoffeectxSetup: the full setup resolves nested per-project inputs and
// generates ~/.coffeecode/config.yaml plus the pi extension.
func TestCoffeectxSetup(t *testing.T) {
	given := map[string]string{
		"projects[0].repoPath":      "/home/me/repo",
		"projects[0].embedProvider": "openai",
		"projects[0].lspCommand":    "typescript-language-server --stdio",
		"projects[0].lspInstall":    "npm i -g typescript-language-server",
		"projects[0].installPiExt":  "true",
		"projects[0].skills":        "api,contract",
	}
	r, err := Resolve(exampleDir("coffeectx-setup"), Opts{Engine: "global", Root: "~"}, given, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	m := byName(r.States)

	cfg, ok := m["coffeecode-config"]
	if !ok {
		t.Fatalf("missing coffeecode-config; got %v", names(r.States))
	}
	if got := cfg.Params["format"]; got != "yaml" {
		t.Errorf("config format = %v, want yaml", got)
	}
	if got, _ := cfg.Params["path"].(string); got != "~/.coffeecode/config.yaml" {
		t.Errorf("config path = %v", got)
	}
	data, _ := cfg.Params["data"].(map[string]any)
	projects, _ := data["projects"].(map[string]any)
	myrepo, _ := projects["myrepo"].(map[string]any)
	if myrepo == nil {
		t.Fatalf("config.projects.myrepo missing; data=%#v", data)
	}
	if got, _ := myrepo["repoPath"].(string); got != "/home/me/repo" {
		t.Errorf("config repoPath = %v", got)
	}
	if _, ok := m["coffeectx-pi-ext"]; !ok {
		t.Errorf("installPiExt=true should emit the pi extension; got %v", names(r.States))
	}
}
