package state

import (
	"context"
	"fmt"
	"regexp"

	"github.com/coffee-code-io/coffeeenv/internal/sys"
)

func init() { Register(&cargoHandler{}) }

// cargoHandler installs a crate via `cargo install`. With a prefix it passes
// `--root <prefix>` (binary in <prefix>/bin); otherwise the default cargo root.
type cargoHandler struct{}

type cargoDesired struct {
	Package string `json:"package"`
	Version string `json:"version"`
	Prefix  string `json:"prefix"`
}

func (cargoHandler) Type() string { return "cargo" }

func (cargoHandler) Decode(rs RawState) (Desired, error) {
	var p cargoDesired
	if err := decodeParams(rs, &p); err != nil {
		return nil, err
	}
	if p.Package == "" {
		return nil, fmt.Errorf("cargo: package is required")
	}
	if p.Version == "" {
		p.Version = "latest"
	}
	return &p, nil
}

func (d *cargoDesired) root() string {
	if d.Prefix == "" {
		return ""
	}
	return sys.ExpandPath(d.Prefix)
}

type cargoObserved struct{ Installed string }

// cargoListLine matches a `cargo install --list` header, e.g. "ripgrep v13.0.0:".
var cargoListLine = regexp.MustCompile(`(?m)^(\S+) v(\S+):`)

func (cargoHandler) Read(ctx context.Context, desired Desired) (Observed, error) {
	d := desired.(*cargoDesired)
	if !sys.Look("cargo") {
		return &cargoObserved{}, nil
	}
	args := []string{"install", "--list"}
	if r := d.root(); r != "" {
		args = append(args, "--root", r)
	}
	res, err := sys.Run(ctx, "cargo", args...)
	if err != nil {
		return nil, err
	}
	for _, m := range cargoListLine.FindAllStringSubmatch(res.Stdout, -1) {
		if m[1] == d.Package {
			return &cargoObserved{Installed: m[2]}, nil
		}
	}
	return &cargoObserved{}, nil
}

func (cargoHandler) Diff(desired Desired, observed Observed) ([]Action, error) {
	d := desired.(*cargoDesired)
	o := observed.(*cargoObserved)
	where := "global"
	if r := d.root(); r != "" {
		where = r
	}
	switch {
	case o.Installed == "":
		return []Action{{StateName: d.Package, Kind: "install-cargo",
			Summary: fmt.Sprintf("cargo install %s@%s (%s)", d.Package, d.Version, where), Payload: *d}}, nil
	case d.Version != "latest" && o.Installed != d.Version:
		return []Action{{StateName: d.Package, Kind: "install-cargo",
			Summary: fmt.Sprintf("cargo update %s: %s -> %s (%s)", d.Package, o.Installed, d.Version, where), Payload: *d}}, nil
	default:
		return nil, nil
	}
}

func (cargoHandler) Apply(ctx context.Context, a Action) error {
	d := a.Payload.(cargoDesired)
	args := []string{"install", d.Package}
	if d.Version != "latest" {
		args = append(args, "--version", d.Version)
	}
	if r := d.root(); r != "" {
		args = append(args, "--root", r)
	}
	return sys.Stream(ctx, "cargo", args...)
}
