package state

import (
	"strings"
	"testing"
)

func TestGoStateDecodeAndDiff(t *testing.T) {
	h := goHandler{}
	d, err := h.Decode(RawState{Type: "go", Params: map[string]any{
		"package": "golang.org/x/tools/gopls", "prefix": "/tmp/v",
	}})
	if err != nil {
		t.Fatal(err)
	}
	gd := d.(*goDesired)
	if gd.Bin != "gopls" {
		t.Errorf("default bin = %q, want gopls", gd.Bin)
	}
	if gd.binDir() != "/tmp/v/bin" {
		t.Errorf("binDir = %q, want /tmp/v/bin", gd.binDir())
	}
	// Absent -> one install action targeting the venv bin dir.
	acts, _ := h.Diff(gd, &goObserved{Present: false})
	if len(acts) != 1 || acts[0].Kind != "install-go" || !strings.Contains(acts[0].Summary, "/tmp/v/bin") {
		t.Errorf("diff = %+v", acts)
	}
	if acts2, _ := h.Diff(gd, &goObserved{Present: true}); len(acts2) != 0 {
		t.Errorf("present should be a no-op, got %+v", acts2)
	}
}

func TestCargoStateDiff(t *testing.T) {
	h := cargoHandler{}
	d, _ := h.Decode(RawState{Type: "cargo", Params: map[string]any{
		"package": "ripgrep", "version": "13.0.0", "prefix": "/tmp/v",
	}})
	cd := d.(*cargoDesired)
	if cd.root() != "/tmp/v" {
		t.Errorf("root = %q", cd.root())
	}
	if acts, _ := h.Diff(cd, &cargoObserved{Installed: ""}); len(acts) != 1 || acts[0].Kind != "install-cargo" {
		t.Errorf("absent -> install; got %+v", acts)
	}
	if acts, _ := h.Diff(cd, &cargoObserved{Installed: "13.0.0"}); len(acts) != 0 {
		t.Errorf("matched version -> no-op; got %+v", acts)
	}
	if acts, _ := h.Diff(cd, &cargoObserved{Installed: "12.0.0"}); len(acts) != 1 {
		t.Errorf("version drift -> update; got %+v", acts)
	}
}

func TestPipStateDiff(t *testing.T) {
	h := pipHandler{}
	// Prefix install: always (re)installs (pip no-ops when satisfied).
	d, _ := h.Decode(RawState{Type: "pip", Params: map[string]any{"package": "black", "prefix": "/tmp/v"}})
	if acts, _ := h.Diff(d, &pipObserved{Unknown: true}); len(acts) != 1 || acts[0].Kind != "install-pip" {
		t.Errorf("prefix -> install; got %+v", acts)
	}
	// Global: matched version is a no-op.
	g, _ := h.Decode(RawState{Type: "pip", Params: map[string]any{"package": "black", "version": "24.0.0"}})
	if acts, _ := h.Diff(g, &pipObserved{Installed: "24.0.0"}); len(acts) != 0 {
		t.Errorf("matched -> no-op; got %+v", acts)
	}
	if acts, _ := h.Diff(g, &pipObserved{Installed: ""}); len(acts) != 1 {
		t.Errorf("absent -> install; got %+v", acts)
	}
}
