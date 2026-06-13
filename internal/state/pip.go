package state

import (
	"context"
	"fmt"
	"regexp"

	"github.com/coffee-code-io/coffeeenv/internal/sys"
)

func init() { Register(&pipHandler{}) }

// pipHandler installs a Python package via `pip install`. With a prefix it
// passes `--prefix <prefix>`; otherwise the active/default environment.
// Idempotency: for the default env `pip show` reports the installed version;
// for a prefix it always (re)installs — `pip install` no-ops when satisfied.
type pipHandler struct{}

type pipDesired struct {
	Package string `json:"package"`
	Version string `json:"version"`
	Prefix  string `json:"prefix"`
}

func (pipHandler) Type() string { return "pip" }

func (pipHandler) Decode(rs RawState) (Desired, error) {
	var p pipDesired
	if err := decodeParams(rs, &p); err != nil {
		return nil, err
	}
	if p.Package == "" {
		return nil, fmt.Errorf("pip: package is required")
	}
	if p.Version == "" {
		p.Version = "latest"
	}
	return &p, nil
}

// pipTool prefers pip3, falling back to pip.
func pipTool() string {
	if sys.Look("pip3") {
		return "pip3"
	}
	return "pip"
}

type pipObserved struct {
	Installed string
	Unknown   bool // prefix install: state not queried, (re)install always
}

var pipShowVersion = regexp.MustCompile(`(?m)^Version:\s*(\S+)`)

func (pipHandler) Read(ctx context.Context, desired Desired) (Observed, error) {
	d := desired.(*pipDesired)
	if d.Prefix != "" {
		return &pipObserved{Unknown: true}, nil
	}
	tool := pipTool()
	if !sys.Look(tool) {
		return &pipObserved{}, nil
	}
	res, err := sys.Run(ctx, tool, "show", d.Package)
	if err != nil {
		return nil, err
	}
	if m := pipShowVersion.FindStringSubmatch(res.Stdout); m != nil {
		return &pipObserved{Installed: m[1]}, nil
	}
	return &pipObserved{}, nil
}

func (pipHandler) Diff(desired Desired, observed Observed) ([]Action, error) {
	d := desired.(*pipDesired)
	o := observed.(*pipObserved)
	where := "global"
	if d.Prefix != "" {
		where = sys.ExpandPath(d.Prefix)
	}
	act := []Action{{StateName: d.Package, Kind: "install-pip",
		Summary: fmt.Sprintf("pip install %s%s (%s)", d.Package, pipVerSuffix(d.Version), where), Payload: *d}}
	switch {
	case o.Unknown, o.Installed == "":
		return act, nil
	case d.Version != "latest" && o.Installed != d.Version:
		act[0].Summary = fmt.Sprintf("pip update %s: %s -> %s (%s)", d.Package, o.Installed, d.Version, where)
		return act, nil
	default:
		return nil, nil
	}
}

func pipVerSuffix(v string) string {
	if v == "latest" {
		return ""
	}
	return "==" + v
}

func (pipHandler) Apply(ctx context.Context, a Action) error {
	d := a.Payload.(pipDesired)
	args := []string{"install", d.Package + pipVerSuffix(d.Version)}
	if d.Prefix != "" {
		args = append(args, "--prefix", sys.ExpandPath(d.Prefix))
	}
	return sys.Stream(ctx, pipTool(), args...)
}
