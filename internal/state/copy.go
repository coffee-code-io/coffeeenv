package state

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/coffee-code-io/coffeeenv/internal/sys"
)

func init() { Register(&copyHandler{}) }

// copyHandler installs a filesystem tree (path-sourced skills/jobs). It copies
// every regular file under src into dst, idempotently: a file is (re)written
// only when missing or differing. src is expected to be absolute (the CUE layer
// resolves a relative src against the chart directory before decode).
type copyHandler struct{}

type copyDesired struct {
	Src string `json:"src"`
	Dst string `json:"dst"`
}

// copyFile is one source->dest pair to materialize.
type copyFile struct {
	srcAbs  string
	dstAbs  string
	content []byte
	mode    os.FileMode
}

type copyObserved struct {
	pending []copyFile // files missing or differing in dst
}

func (copyHandler) Type() string { return "copy" }

func (copyHandler) Decode(rs RawState) (Desired, error) {
	var p copyDesired
	if err := decodeParams(rs, &p); err != nil {
		return nil, err
	}
	if p.Src == "" || p.Dst == "" {
		return nil, errors.New("copy: src and dst are required")
	}
	return &p, nil
}

func (copyHandler) Read(_ context.Context, desired Desired) (Observed, error) {
	d := desired.(*copyDesired)
	src := sys.ExpandPath(d.Src)
	dst := sys.ExpandPath(d.Dst)

	info, err := os.Stat(src)
	if err != nil {
		return nil, fmt.Errorf("copy: src %q: %w", src, err)
	}

	// Enumerate source files as (srcAbs, dstAbs). A file src copies to
	// dst/<basename>; a dir src mirrors its tree under dst.
	var planned []copyFile
	add := func(srcAbs, dstAbs string, mode os.FileMode) error {
		b, err := os.ReadFile(srcAbs)
		if err != nil {
			return err
		}
		planned = append(planned, copyFile{srcAbs: srcAbs, dstAbs: dstAbs, content: b, mode: mode})
		return nil
	}
	if info.IsDir() {
		if err := filepath.WalkDir(src, func(p string, de fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if de.IsDir() {
				// Never install coffeeenv-internal scaffolding (defensive: a skill
				// chart carries no cue.mod, but other copy sources might).
				if p != src && de.Name() == "cue.mod" {
					return fs.SkipDir
				}
				return nil
			}
			// Skip reserved coffeeenv metadata so a pulled skill's lock/manifest
			// don't leak into the agent's skills dir.
			if de.Name() == "coffeeenv.lock.json" || de.Name() == "manifest.json" {
				return nil
			}
			rel, err := filepath.Rel(src, p)
			if err != nil {
				return err
			}
			fi, err := de.Info()
			if err != nil {
				return err
			}
			return add(p, filepath.Join(dst, rel), fi.Mode().Perm())
		}); err != nil {
			return nil, fmt.Errorf("copy: walk %q: %w", src, err)
		}
	} else {
		if err := add(src, filepath.Join(dst, filepath.Base(src)), info.Mode().Perm()); err != nil {
			return nil, err
		}
	}

	// Keep only the files that are missing or differ.
	obs := &copyObserved{}
	for _, cf := range planned {
		existing, err := os.ReadFile(cf.dstAbs)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				obs.pending = append(obs.pending, cf)
				continue
			}
			return nil, err
		}
		if sys.HashBytes(existing) != sys.HashBytes(cf.content) {
			obs.pending = append(obs.pending, cf)
		}
	}
	return obs, nil
}

func (copyHandler) Diff(desired Desired, observed Observed) ([]Action, error) {
	d := desired.(*copyDesired)
	o := observed.(*copyObserved)
	var acts []Action
	for _, cf := range o.pending {
		acts = append(acts, Action{
			StateName: d.Dst,
			Kind:      "copy-file",
			Summary:   fmt.Sprintf("copy %s -> %s", cf.srcAbs, cf.dstAbs),
			Payload:   filePayload{path: cf.dstAbs, content: cf.content, mode: cf.mode},
		})
	}
	return acts, nil
}

func (copyHandler) Apply(_ context.Context, a Action) error {
	p := a.Payload.(filePayload)
	return sys.WriteFileAtomic(p.path, p.content, p.mode)
}
