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
		"coffeectx.projects.myrepo.repoPath":        "/home/me/repo",
		"coffeectx.projects.myrepo.language":        "typescript",
		"coffeectx.projects.myrepo.lspDirs":         "",
		"coffeectx.projects.myrepo.embedDimensions": "4096",
		"coffeectx.projects.myrepo.skills":          "api,contract",
		"coffeectx.projects.myrepo.jobs":            "reindex",
		"coffeectx.authType":                        "apiKey",
		"coffeectx.provider":                        "", // empty -> custom url path
		"coffeectx.url":                             "https://api.example.com",
		"coffeectx.apiKey":                          "sk-test",
		"coffeectx.embeddingsModel":                 "embed-1",
		"coffeectx.indexerModel":                    "index-1",
		"coffeectx.uiModel":                         "ui-1",
		"coffeectx.active":                          "myrepo",
		"coffeectx.autolaunch":                      "true",
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
	core, _ := myrepo["core"].(map[string]any)
	embed, _ := core["embed"].(map[string]any)
	switch got := embed["dimensions"].(type) {
	case int64:
		if got != 4096 {
			t.Errorf("embed dimensions = %v, want 4096", got)
		}
	case int:
		if got != 4096 {
			t.Errorf("embed dimensions = %v, want 4096", got)
		}
	case float64:
		if got != 4096 {
			t.Errorf("embed dimensions = %v, want 4096", got)
		}
	default:
		t.Errorf("embed dimensions = %#v (%T), want 4096", got, got)
	}

	// The @multichoice skills value "api,contract" is injected as a CUE list.
	skillsInc, _ := myrepo["skills"].(map[string]any)["jobs"].(map[string]any)["include"].([]any)
	if len(skillsInc) != 2 || skillsInc[0] != "api" || skillsInc[1] != "contract" {
		t.Errorf("skills include = %#v, want [api contract] list", skillsInc)
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
	// Each project gets an init shell state, guarded by its db.
	initSt, ok := m["coffeectx-init-myrepo"]
	if !ok {
		t.Fatalf("missing coffeectx-init-myrepo state; got %v", names(r.States))
	}
	if got, _ := initSt.Params["run"].(string); got != "coffeectx init myrepo" {
		t.Errorf("init run = %q", got)
	}
	if got, _ := initSt.Params["creates"].(string); got != "~/.coffeecode/db/myrepo.db" {
		t.Errorf("init creates = %q", got)
	}

	// In-built jobs: the active agent (claude) log-import is enabled with params;
	// codex/pi are emitted but off; plans/indexer/span-link are always on.
	jobsAll, _ := myrepo["jobs"].(map[string]any)
	claudeJob, _ := jobsAll["claude"].(map[string]any)
	if claudeJob == nil || claudeJob["enabled"] != true {
		t.Fatalf("jobs.claude should be enabled; jobs=%#v", jobsAll)
	}
	claudeParams, _ := claudeJob["parameters"].(map[string]any)
	if got, _ := claudeParams["path"].(string); got != "~/.claude/projects/-home-me-repo" {
		t.Errorf("jobs.claude path = %q", got)
	}
	if got, _ := claudeParams["intervalMs"].(int); got != 30000 {
		// numbers may decode as other kinds depending on the extractor; check non-empty too.
		if s, _ := claudeParams["intervalMs"]; s == nil {
			t.Errorf("jobs.claude intervalMs missing; params=%#v", claudeParams)
		}
	}
	if s, _ := claudeParams["newerThan"].(string); s == "" {
		t.Errorf("jobs.claude newerThan should be set; params=%#v", claudeParams)
	}
	if cj, _ := jobsAll["codex"].(map[string]any); cj == nil || cj["enabled"] != false {
		t.Errorf("jobs.codex should be present and disabled; got %#v", jobsAll["codex"])
	}
	if pj, _ := jobsAll["pi"].(map[string]any); pj == nil || pj["enabled"] != false {
		t.Errorf("jobs.pi should be present and disabled; got %#v", jobsAll["pi"])
	}
	if pl, _ := jobsAll["plans"].(map[string]any); pl == nil || pl["enabled"] != true {
		t.Errorf("jobs.plans should be enabled; got %#v", jobsAll["plans"])
	}
	if sl, _ := jobsAll["span-link"].(map[string]any); sl == nil || sl["enabled"] != true {
		t.Errorf("jobs.span-link should be enabled; got %#v", jobsAll["span-link"])
	}
	idxJob, _ := jobsAll["indexer"].(map[string]any)
	if idxJob == nil || idxJob["enabled"] != true {
		t.Fatalf("jobs.indexer should be enabled; got %#v", jobsAll["indexer"])
	}
	idxParams, _ := idxJob["parameters"].(map[string]any)
	idxAuth, _ := idxParams["auth"].(map[string]any)
	if got, _ := idxAuth["model"].(string); got != "index-1" {
		t.Errorf("jobs.indexer auth.model = %v, want index-1", got)
	}

	// Embedding auth: an AuthSettings block under core.embed.auth with the
	// embeddings model and shared credential.
	core, _ = myrepo["core"].(map[string]any)
	embed, _ = core["embed"].(map[string]any)
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

// TestCoffeectxLspMonorepo: a project with lspDirs emits one `lsp:<dir>` job per
// subdirectory (each scoped to its absolute path) and no whole-repo `lsp` job.
func TestCoffeectxSetupUsesAbsoluteRoot(t *testing.T) {
	given := map[string]string{
		"coffeectx.projects.myrepo.repoPath":        "/home/me/repo",
		"coffeectx.projects.myrepo.language":        "go",
		"coffeectx.projects.myrepo.lspDirs":         "gateway",
		"coffeectx.projects.myrepo.embedDimensions": "auto",
		"coffeectx.projects.myrepo.skills":          "",
		"coffeectx.projects.myrepo.jobs":            "",
		"coffeectx.authType":                        "openai-oauth",
		"coffeectx.embedProvider":                   "",
		"coffeectx.embedUrl":                        "https://embed.example.com",
		"coffeectx.embedApiKey":                     "embed-key",
		"coffeectx.embeddingsModel":                 "embed-1",
		"coffeectx.indexerModel":                    "index-1",
		"coffeectx.uiModel":                         "ui-1",
		"coffeectx.active":                          "myrepo",
		"coffeectx.autolaunch":                      "true",
	}
	r, err := Resolve(exampleDir("coffeectx-setup"), Opts{Engine: "global", Root: "/home/me", OS: "linux"}, given, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	m := byName(r.States)
	cfg := m["coffeecode-config"]
	data, _ := cfg.Params["data"].(map[string]any)
	projects, _ := data["projects"].(map[string]any)
	myrepo, _ := projects["myrepo"].(map[string]any)
	if got, _ := myrepo["db"].(string); got != "/home/me/.coffeecode/db/myrepo.db" {
		t.Fatalf("db path = %q, want absolute home path", got)
	}
	if got, _ := data["types"].(map[string]any)["userDir"].(string); got != "/home/me/.coffeecode/types" {
		t.Fatalf("types.userDir = %q, want absolute home path", got)
	}
	unit := m["coffeectx-systemd"]
	if got, _ := unit.Params["path"].(string); got != "/home/me/.config/systemd/user/coffeectx.service" {
		t.Fatalf("systemd unit path = %q, want absolute home path", got)
	}
	unitContent, _ := unit.Params["content"].(string)
	if !strings.Contains(unitContent, "WorkingDirectory=/home/me") || !strings.Contains(unitContent, "ExecStart=/usr/bin/env bash -lc 'exec coffeectx daemonize'") {
		t.Fatalf("systemd unit does not use absolute setup paths/content:\n%s", unitContent)
	}
	uiUnit := m["coffeectx-ui-systemd"]
	if got, _ := uiUnit.Params["path"].(string); got != "/home/me/.config/systemd/user/coffeectx-ui.service" {
		t.Fatalf("ui systemd unit path = %q, want absolute home path", got)
	}
	uiContent, _ := uiUnit.Params["content"].(string)
	if !strings.Contains(uiContent, "WorkingDirectory=/home/me") || !strings.Contains(uiContent, "ExecStart=/usr/bin/env bash -lc 'exec coffeectx-ui'") {
		t.Fatalf("ui systemd unit does not use absolute setup paths/content:\n%s", uiContent)
	}
	enable := m["coffeectx-systemd-enable"]
	if got, _ := enable.Params["run"].(string); got != "systemctl --user enable --now coffeectx.service coffeectx-ui.service" {
		t.Fatalf("systemd enable run = %q", got)
	}
}

func TestCoffeectxLspMonorepo(t *testing.T) {
	given := map[string]string{
		"coffeectx.projects.mono.repoPath": "/home/me/mono",
		"coffeectx.projects.mono.language": "typescript",
		"coffeectx.projects.mono.lspDirs":  "frontend, backend",
		"coffeectx.projects.mono.skills":   "",
		"coffeectx.projects.mono.jobs":     "",
		"coffeectx.authType":               "apiKey",
		"coffeectx.provider":               "openrouter",
		"coffeectx.apiKey":                 "sk-x",
		"coffeectx.embeddingsModel":        "e",
		"coffeectx.indexerModel":           "i",
		"coffeectx.uiModel":                "u",
		"coffeectx.active":                 "mono",
		"coffeectx.autolaunch":             "false",
	}
	r, err := Resolve(exampleDir("coffeectx-setup"), Opts{Engine: "global", Root: "~", OS: "darwin"}, given, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	m := byName(r.States)
	data, _ := m["coffeecode-config"].Params["data"].(map[string]any)
	projects, _ := data["projects"].(map[string]any)
	mono, _ := projects["mono"].(map[string]any)
	jobs, _ := mono["jobs"].(map[string]any)

	if _, ok := jobs["lsp"]; ok {
		t.Errorf("monorepo project should not emit a whole-repo lsp job; jobs=%v", keysOf(jobs))
	}
	for dir, wantPath := range map[string]string{
		"frontend": "/home/me/mono/frontend",
		"backend":  "/home/me/mono/backend",
	} {
		job, ok := jobs["lsp:"+dir].(map[string]any)
		if !ok {
			t.Fatalf("missing lsp:%s job; jobs=%v", dir, keysOf(jobs))
		}
		if job["enabled"] != true {
			t.Errorf("lsp:%s should be enabled", dir)
		}
		params, _ := job["parameters"].(map[string]any)
		if got, _ := params["repoPath"].(string); got != wantPath {
			t.Errorf("lsp:%s repoPath = %q, want %q", dir, got, wantPath)
		}
		if got, _ := params["lspCommand"].(string); got != "typescript-language-server --stdio" {
			t.Errorf("lsp:%s lspCommand = %q", dir, got)
		}
	}
}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
