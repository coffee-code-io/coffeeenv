package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/coffee-code-io/coffeeenv/internal/sys"
)

func init() { Register(&goHandler{}) }

// goHandler installs a Go binary via `go install package@version`. With a
// prefix the binary lands in <prefix>/bin (via GOBIN); otherwise the default
// GOBIN. Idempotency is best-effort: present-binary => skip (version is not
// re-verified; reinstalling is safe).
type goHandler struct{}

type goDesired struct {
	Package string `json:"package"`
	Version string `json:"version"`
	Prefix  string `json:"prefix"`
	Bin     string `json:"bin"`
}

func (goHandler) Type() string { return "go" }

func (goHandler) Decode(rs RawState) (Desired, error) {
	var p goDesired
	if err := decodeParams(rs, &p); err != nil {
		return nil, err
	}
	if p.Package == "" {
		return nil, fmt.Errorf("go: package is required")
	}
	if p.Version == "" {
		p.Version = "latest"
	}
	if p.Bin == "" {
		pkg := p.Package
		if i := strings.Index(pkg, "@"); i >= 0 {
			pkg = pkg[:i]
		}
		p.Bin = filepath.Base(strings.TrimSuffix(pkg, "/..."))
	}
	return &p, nil
}

// binDir is <prefix>/bin (expanded), or "" for the default GOBIN.
func (d *goDesired) binDir() string {
	if d.Prefix == "" {
		return ""
	}
	return filepath.Join(sys.ExpandPath(d.Prefix), "bin")
}

type goObserved struct{ Present bool }

func (goHandler) Read(_ context.Context, desired Desired) (Observed, error) {
	d := desired.(*goDesired)
	dir := d.binDir()
	if dir == "" {
		return &goObserved{Present: sys.Look(d.Bin)}, nil
	}
	_, err := os.Stat(filepath.Join(dir, d.Bin))
	return &goObserved{Present: err == nil}, nil
}

func (goHandler) Diff(desired Desired, observed Observed) ([]Action, error) {
	d := desired.(*goDesired)
	if observed.(*goObserved).Present {
		return nil, nil
	}
	where := "global"
	if dir := d.binDir(); dir != "" {
		where = dir
	}
	return []Action{{
		StateName: d.Package,
		Kind:      "install-go",
		Summary:   fmt.Sprintf("go install %s@%s (%s)", d.Package, d.Version, where),
		Payload:   *d,
	}}, nil
}

func (goHandler) Apply(ctx context.Context, a Action) error {
	d := a.Payload.(goDesired)
	var env []string
	if dir := d.binDir(); dir != "" {
		env = append(env, "GOBIN="+dir)
	}
	return sys.StreamEnv(ctx, env, "go", "install", d.Package+"@"+d.Version)
}
