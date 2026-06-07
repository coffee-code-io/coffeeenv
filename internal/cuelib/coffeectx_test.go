package cuelib

import (
	"strings"
	"testing"
)

// TestCoffeectxInstall: with confirm=true the module installs @coffeectx/server,
// registers the MCP server (non-pi agent), and adds the CoffeeCtx paragraph (via
// the Claude renderer).
func TestCoffeectxInstall(t *testing.T) {
	given := map[string]string{"confirm": "true"}
	r, err := Resolve(exampleDir("claude-coffeectx"), Opts{Engine: "global", Root: "~"}, given, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	raws := r.States
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
		"projects[0].repoPath":  "/home/me/repo",
		"projects[0].language":  "typescript",
		"projects[0].skills":    "api,contract",
		"projects[0].jobs":      "reindex",
		"input.confirm":         "true",
		"input.apiKey":          "sk-test",
		"input.baseUrl":         "https://api.example.com",
		"input.embeddingsModel": "embed-1",
		"input.indexerModel":    "index-1",
		"input.uiModel":         "ui-1",
		"input.autolaunch":      "true",
	}
	r, err := Resolve(exampleDir("coffeectx-setup"), Opts{Engine: "global", Root: "~", OS: "darwin"}, given, nil)
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

	// Global auth + models land in the config.
	auth, _ := data["auth"].(map[string]any)
	if got, _ := auth["key"].(string); got != "sk-test" {
		t.Errorf("config auth.key = %v", got)
	}
	models, _ := data["models"].(map[string]any)
	if got, _ := models["embeddings"].(string); got != "embed-1" {
		t.Errorf("config models.embeddings = %v", got)
	}

	// A claude (non-pi) agent registers the MCP and does NOT emit the pi extension.
	if _, ok := m["coffeectx-pi-ext"]; ok {
		t.Errorf("non-pi agent should not emit the pi extension; got %v", names(r.States))
	}

	// Enabled skills are installed into the coffeecode skill dir, jobs into the job dir.
	if _, ok := m["coffeecode-job-reindex"]; !ok {
		t.Errorf("registered job should be installed to ~/.coffeecode/jobs; got %v", names(r.States))
	}

	// autolaunch=true on a darwin global install writes the launchd plist.
	if _, ok := m["coffeectx-launchd"]; !ok {
		t.Errorf("autolaunch on darwin should emit the launchd plist; got %v", names(r.States))
	}
}
