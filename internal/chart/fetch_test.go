package chart

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPullLocal(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "env.cue"), []byte("package env\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Setenv("COFFEEENV_ROOT", root)

	c, err := Open("x")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{src, "local://" + src} {
		ref, commit, digest, err := c.Pull(context.Background(), source)
		if err != nil {
			t.Fatalf("pull %q: %v", source, err)
		}
		if ref != "" || commit != "" || digest != "" {
			t.Errorf("local pull should have no ref/commit/digest, got %q/%q/%q", ref, commit, digest)
		}
		if _, err := os.Stat(filepath.Join(c.Dir, "env.cue")); err != nil {
			t.Errorf("pulled chart missing env.cue: %v", err)
		}
		if _, err := os.Stat(c.CueModule()); err != nil {
			t.Errorf("pulled chart missing cue.mod: %v", err)
		}
	}
}

func TestSchemeRecognition(t *testing.T) {
	for _, s := range []string{"git+https://h/r.git", "git+ssh://git@h/r.git", "git@h:r.git", "https://h/r.git"} {
		if !isGitSource(s) {
			t.Errorf("%q should be a git source", s)
		}
	}
	if !isOCISource("oci://reg/repo:tag") {
		t.Errorf("oci:// should be an OCI source")
	}
	if isGitSource("local:///tmp/x") || isOCISource("local:///tmp/x") {
		t.Errorf("local:// misclassified")
	}
}
