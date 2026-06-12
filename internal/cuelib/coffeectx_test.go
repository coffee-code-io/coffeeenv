package cuelib

import (
	"strings"
	"testing"
)

// TestCoffeectxInstall: embedding coffeectx.#Mcp (no confirmation) always
// installs the indexer, installs @coffeectx/server and registers the MCP server
// for the non-pi Claude agent, and adds the CoffeeCtx paragraph.
func TestCoffeectxInstall(t *testing.T) {
	r, err := Resolve(exampleDir("claude-coffeectx"), Opts{Engine: "global", Root: "~"}, nil, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	raws := r.States
	m := byName(raws)
	if _, ok := m["coffeectx-indexer"]; !ok {
		t.Errorf("indexer should always be installed; got %v", names(raws))
	}
	if _, ok := m["coffeectx-server"]; !ok {
		t.Errorf("missing @coffeectx/server npm state (non-pi agent); got %v", names(raws))
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
		"coffeectx.projects.myrepo.repoPath": "/home/me/repo",
		"coffeectx.projects.myrepo.language": "typescript",
		"coffeectx.projects.myrepo.skills":   "api,contract",
		"coffeectx.projects.myrepo.jobs":     "reindex",
		"coffeectx.authType":                 "apiKey",
		"coffeectx.provider":                 "", // empty -> custom url path
		"coffeectx.url":                      "https://api.example.com",
		"coffeectx.apiKey":                   "sk-test",
		"coffeectx.embeddingsModel":          "embed-1",
		"coffeectx.indexerModel":             "index-1",
		"coffeectx.uiModel":                  "ui-1",
		"coffeectx.active":                   "myrepo",
		"coffeectx.autolaunch":               "true",
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
	if got, _ := data["active"].(string); got != "myrepo" {
		t.Errorf("config active = %v, want myrepo", got)
	}

	// The project's language resolves to the lsp command (from lsp.available) and
	// the server is installed for that language.
	jobsCfg, _ := myrepo["jobs"].(map[string]any)
	lspJob, _ := jobsCfg["lsp"].(map[string]any)
	lspParams, _ := lspJob["parameters"].(map[string]any)
	if got, _ := lspParams["lspCommand"].(string); got != "typescript-language-server --stdio" {
		t.Errorf("lspCommand = %v", got)
	}
	if _, ok := m["lsp-install-typescript"]; !ok {
		t.Errorf("missing lsp-install-typescript state; got %v", names(r.States))
	}

	// Embedding auth: an AuthSettings block under core.embed.auth with the
	// embeddings model and shared credential.
	core, _ := myrepo["core"].(map[string]any)
	embed, _ := core["embed"].(map[string]any)
	embedAuth, _ := embed["auth"].(map[string]any)
	if got, _ := embedAuth["authType"].(string); got != "apiKey" {
		t.Errorf("embed auth.authType = %v", got)
	}
	if got, _ := embedAuth["url"].(string); got != "https://api.example.com" {
		t.Errorf("embed auth.url = %v", got)
	}
	if got, _ := embedAuth["apiKey"].(string); got != "sk-test" {
		t.Errorf("embed auth.apiKey = %v", got)
	}
	if got, _ := embedAuth["model"].(string); got != "embed-1" {
		t.Errorf("embed auth.model = %v", got)
	}

	// UI agent auth: agent.auth carries the ui model.
	agent, _ := myrepo["agent"].(map[string]any)
	agentAuth, _ := agent["auth"].(map[string]any)
	if got, _ := agentAuth["model"].(string); got != "ui-1" {
		t.Errorf("agent auth.model = %v", got)
	}

	// Enabled job carries the indexer model in parameters.auth.
	jobs, _ := myrepo["jobs"].(map[string]any)
	reindex, _ := jobs["reindex"].(map[string]any)
	if reindex == nil {
		t.Fatalf("jobs.reindex missing; jobs=%#v", jobs)
	}
	if got, _ := reindex["enabled"].(bool); !got {
		t.Errorf("jobs.reindex.enabled = %v, want true", reindex["enabled"])
	}
	params, _ := reindex["parameters"].(map[string]any)
	jobAuth, _ := params["auth"].(map[string]any)
	if got, _ := jobAuth["model"].(string); got != "index-1" {
		t.Errorf("jobs.reindex parameters.auth.model = %v", got)
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
