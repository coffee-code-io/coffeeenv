package state

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/coffee-code-io/coffeeenv/internal/sys"
)

func init() { Register(&lnHandler{}) }

type lnHandler struct{}

type lnDesired struct {
	Src      string `json:"src"`
	Dst      string `json:"dst"`
	Soft     *bool  `json:"soft"`
	Force    *bool  `json:"force"`
	Sudo     *bool  `json:"sudo"`
	MkdirAll *bool  `json:"mkdir_all"`
	DirPerm  uint32 `json:"dir_perm"`
}

type lnObserved struct {
	SrcAbs string
	DstAbs string
	OK     bool
	Exists bool
}

type lnPayload struct {
	src      string
	dst      string
	soft     bool
	force    bool
	sudo     bool
	mkdirAll bool
	dirPerm  os.FileMode
}

func (lnHandler) Type() string { return "ln" }

func (lnHandler) Decode(rs RawState) (Desired, error) {
	var p lnDesired
	if err := decodeParams(rs, &p); err != nil {
		return nil, err
	}
	if p.Src == "" || p.Dst == "" {
		return nil, errors.New("ln: src and dst are required")
	}
	if p.Soft == nil {
		soft := true
		p.Soft = &soft
	}
	if p.Force == nil {
		force := true
		p.Force = &force
	}
	if p.Sudo == nil {
		sudo := false
		p.Sudo = &sudo
	}
	if p.MkdirAll == nil {
		mkdirAll := true
		p.MkdirAll = &mkdirAll
	}
	if p.DirPerm == 0 {
		p.DirPerm = 0o755
	}
	return &p, nil
}

func (lnHandler) Read(_ context.Context, desired Desired) (Observed, error) {
	d := desired.(*lnDesired)
	src := sys.ExpandPath(d.Src)
	dst := sys.ExpandPath(d.Dst)
	obs := &lnObserved{SrcAbs: src, DstAbs: dst}
	if *d.Soft {
		target, err := os.Readlink(dst)
		if err == nil {
			obs.Exists = true
			obs.OK = target == src
			return obs, nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			return obs, nil
		}
		if _, statErr := os.Lstat(dst); statErr == nil {
			obs.Exists = true
			return obs, nil
		}
		return nil, err
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return obs, nil
		}
		return nil, err
	}
	obs.Exists = true
	srcInfo, err := os.Stat(src)
	if err != nil {
		return obs, nil
	}
	obs.OK = os.SameFile(srcInfo, dstInfo)
	return obs, nil
}

func (lnHandler) Diff(desired Desired, observed Observed) ([]Action, error) {
	d := desired.(*lnDesired)
	o := observed.(*lnObserved)
	if o.OK {
		return nil, nil
	}
	if o.Exists && !*d.Force {
		return nil, fmt.Errorf("ln: %s exists and force is false", o.DstAbs)
	}
	verb := "symlink"
	if !*d.Soft {
		verb = "link"
	}
	return []Action{{
		StateName: d.Dst,
		Kind:      "link",
		Summary:   fmt.Sprintf("%s %s -> %s", verb, o.DstAbs, o.SrcAbs),
		Payload:   lnPayload{src: o.SrcAbs, dst: o.DstAbs, soft: *d.Soft, force: *d.Force, sudo: *d.Sudo, mkdirAll: *d.MkdirAll, dirPerm: os.FileMode(d.DirPerm)},
	}}, nil
}

func (lnHandler) Apply(_ context.Context, a Action) error {
	p := a.Payload.(lnPayload)
	if p.sudo {
		if p.mkdirAll {
			if err := exec.Command("sudo", "mkdir", "-p", filepath.Dir(p.dst)).Run(); err != nil {
				return err
			}
			if err := exec.Command("sudo", "chmod", fmt.Sprintf("%#o", p.dirPerm), filepath.Dir(p.dst)).Run(); err != nil {
				return err
			}
		}
		args := []string{}
		if p.soft {
			args = append(args, "-s")
		}
		if p.force {
			args = append(args, "-f")
		}
		args = append(args, p.src, p.dst)
		return exec.Command("sudo", append([]string{"ln"}, args...)...).Run()
	}
	if p.mkdirAll {
		if err := os.MkdirAll(filepath.Dir(p.dst), p.dirPerm); err != nil {
			return err
		}
		if err := os.Chmod(filepath.Dir(p.dst), p.dirPerm); err != nil {
			return err
		}
	}
	if p.force {
		if err := os.Remove(p.dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if p.soft {
		return os.Symlink(p.src, p.dst)
	}
	return os.Link(p.src, p.dst)
}
