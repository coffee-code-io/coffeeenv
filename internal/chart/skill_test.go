package chart

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestPullSkill pulls a local:// skill directory (SKILL.md, no CUE) and verifies
// it lands as a type:"skill" chart with the skill files but no cue.mod.
func TestPullSkill(t *testing.T) {
	t.Setenv("COFFEEENV_ROOT", t.TempDir())

	// A skill source: SKILL.md + a supporting file.
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "SKILL.md"), "---\nname: pdf\ndescription: Work with PDFs\n---\n\nDo PDF things.")
	mustWrite(t, filepath.Join(src, "scripts", "run.sh"), "echo hi\n")

	c, err := Open("pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := c.PullSkill(context.Background(), "local://"+src); err != nil {
		t.Fatalf("PullSkill: %v", err)
	}

	m, ok, err := c.ReadManifest()
	if err != nil || !ok {
		t.Fatalf("ReadManifest ok=%v err=%v", ok, err)
	}
	if m.Type != "skill" {
		t.Errorf("manifest type = %q, want skill", m.Type)
	}
	if m.Module != "coffeeenv.dev/skill/pdf" {
		t.Errorf("manifest module = %q", m.Module)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "scripts", "run.sh")); err != nil {
		t.Errorf("supporting file not copied: %v", err)
	}
	if _, err := os.Stat(c.CueModule()); err == nil {
		t.Errorf("skill should not have a cue.mod")
	}
}

// TestPullSkillRejectsNonSkill: a source without SKILL.md is rejected.
func TestPullSkillRejectsNonSkill(t *testing.T) {
	t.Setenv("COFFEEENV_ROOT", t.TempDir())
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "README.md"), "not a skill")
	c, _ := Open("x")
	if _, _, _, err := c.PullSkill(context.Background(), "local://"+src); err == nil {
		t.Fatal("expected error for source without SKILL.md")
	}
}

func TestIsSource(t *testing.T) {
	cases := map[string]bool{
		"claude-basic":                       false,
		"pdf":                                false,
		"git+https://h/u/r.git#main:skills/x": true,
		"git@h:u/r.git":                      true,
		"oci://reg/x:1":                      true,
		"local://./skills/pdf":               true,
		"./examples/pi-basic":                true,
		"/abs/path":                          true,
		"u/r":                                true,
	}
	for in, want := range cases {
		if got := IsSource(in); got != want {
			t.Errorf("IsSource(%q) = %v, want %v", in, got, want)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
