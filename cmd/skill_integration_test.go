package cmd

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/coffee-code-io/coffeeenv/internal/chart"
	"github.com/coffee-code-io/coffeeenv/internal/venv"
)

// TestApplyAcceptsSourceArg: a chart arg that is a git/oci/local source is pulled
// (deduped) and composed, instead of requiring a prior `pull`.
func TestApplyAcceptsSourceArg(t *testing.T) {
	t.Setenv("COFFEEENV_ROOT", t.TempDir())
	src := "local://" + exampleDir("claude-basic")

	tgt, err := resolveTarget(context.Background(), src, "", "", nil)
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if len(tgt.manifest.Execs) != 1 || tgt.manifest.Execs[0] != "coffeeenv.dev/examples/claude-basic" {
		t.Fatalf("execs = %v, want [claude-basic module]", tgt.manifest.Execs)
	}
	c, _ := chart.Open("claude-basic")
	if !c.Exists() {
		t.Fatalf("source arg should have pulled the chart")
	}
	// It composes into a plan.
	if _, _, err := computePlan(context.Background(), tgt, nil); err != nil {
		t.Fatalf("computePlan: %v", err)
	}
	// A second resolve doesn't fail (chart already pulled — dedup).
	if _, err := resolveTarget(context.Background(), src, "", "", nil); err != nil {
		t.Fatalf("second resolveTarget: %v", err)
	}
}

// TestApplySkillAddsSkill: apply-skill pulls a skill, accumulates it into the
// venv manifest's skills (deduped), and the composed plan installs it into the
// active agent's skills dir.
func TestApplySkillAddsSkill(t *testing.T) {
	t.Setenv("COFFEEENV_ROOT", t.TempDir())

	// A local skill source with a fixed dir name (becomes the skill name).
	skillDir := filepath.Join(t.TempDir(), "mypdf")
	writeTmp(t, filepath.Join(skillDir, "SKILL.md"), "---\nname: mypdf\ndescription: PDFs\n---\n\nbody")

	v, _ := venv.Open("v")
	if err := v.Create(); err != nil {
		t.Fatal(err)
	}

	// Apply an agent exec (claude) into the venv so there's a renderer, persist.
	tgt, err := resolveTarget(context.Background(), "local://"+exampleDir("claude-basic"), "v", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	tgt.manifest.Values = map[string]string{}
	if err := tgt.save(tgt.manifest); err != nil {
		t.Fatal(err)
	}

	// Add the skill.
	st, err := resolveSkillTarget(context.Background(), "local://"+skillDir, "v", nil)
	if err != nil {
		t.Fatalf("resolveSkillTarget: %v", err)
	}
	if len(st.manifest.Skills) != 1 || st.manifest.Skills[0] != "mypdf" {
		t.Fatalf("skills = %v, want [mypdf]", st.manifest.Skills)
	}
	if err := st.save(st.manifest); err != nil {
		t.Fatal(err)
	}

	// Re-adding dedups.
	st2, err := resolveSkillTarget(context.Background(), "local://"+skillDir, "v", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(st2.manifest.Skills) != 1 {
		t.Errorf("re-add should dedup skills, got %v", st2.manifest.Skills)
	}

	// The composed plan installs the skill into the claude skills dir.
	p, _, err := computePlan(context.Background(), st2, nil)
	if err != nil {
		t.Fatalf("computePlan: %v", err)
	}
	want := filepath.Join("skills", "mypdf")
	var found bool
	for _, a := range p.Actions {
		if a.Kind == "copy-file" && containsSub(a.Summary, want) && containsSub(a.Summary, "SKILL.md") {
			found = true
		}
	}
	if !found {
		t.Errorf("plan should install the skill under %s; actions=%v", want, p.Actions)
	}
}
