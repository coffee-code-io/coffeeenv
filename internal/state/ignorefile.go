package state

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/coffee-code-io/coffeeenv/internal/sys"
)

func init() { Register(&ignoreFileHandler{}) }

type ignoreFileHandler struct{}

type ignoreFileDesired struct {
	Path     string   `json:"path"`
	Line     string   `json:"line"`
	Lines    []string `json:"lines"`
	MkdirAll *bool    `json:"mkdir_all"`
	DirPerm  uint32   `json:"dir_perm"`
}

type ignoreFileObserved struct {
	AbsPath string
	Present map[string]bool
}

type ignoreFilePayload struct {
	path     string
	lines    []string
	mkdirAll bool
	dirPerm  os.FileMode
}

func (ignoreFileHandler) Type() string { return "ignorefile" }

func (ignoreFileHandler) Decode(rs RawState) (Desired, error) {
	var p ignoreFileDesired
	if err := decodeParams(rs, &p); err != nil {
		return nil, err
	}
	if p.Path == "" {
		return nil, errors.New("ignorefile: path is required")
	}
	if p.Line != "" && len(p.Lines) > 0 {
		return nil, errors.New("ignorefile: set either line or lines, not both")
	}
	if p.Line == "" && len(p.Lines) == 0 {
		return nil, errors.New("ignorefile: line or lines is required")
	}
	for _, line := range p.allLines() {
		if line == "" {
			return nil, errors.New("ignorefile: empty lines are not supported")
		}
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

func (d *ignoreFileDesired) allLines() []string {
	if d.Line != "" {
		return []string{d.Line}
	}
	return d.Lines
}

func (ignoreFileHandler) Read(_ context.Context, desired Desired) (Observed, error) {
	d := desired.(*ignoreFileDesired)
	abs := sys.ExpandPath(d.Path)
	obs := &ignoreFileObserved{AbsPath: abs, Present: map[string]bool{}}
	b, err := os.ReadFile(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return obs, nil
		}
		return nil, err
	}
	s := bufio.NewScanner(bytes.NewReader(b))
	for s.Scan() {
		obs.Present[s.Text()] = true
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return obs, nil
}

func (ignoreFileHandler) Diff(desired Desired, observed Observed) ([]Action, error) {
	d := desired.(*ignoreFileDesired)
	o := observed.(*ignoreFileObserved)
	var missing []string
	for _, line := range d.allLines() {
		if !o.Present[line] {
			missing = append(missing, line)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	return []Action{{
		StateName: d.Path,
		Kind:      "append-ignore-lines",
		Summary:   fmt.Sprintf("append %d ignore line(s) to %s", len(missing), o.AbsPath),
		Payload:   ignoreFilePayload{path: o.AbsPath, lines: missing, mkdirAll: *d.MkdirAll, dirPerm: os.FileMode(d.DirPerm)},
	}}, nil
}

func (ignoreFileHandler) Apply(_ context.Context, a Action) error {
	p := a.Payload.(ignoreFilePayload)
	if p.mkdirAll {
		if err := os.MkdirAll(filepath.Dir(p.path), p.dirPerm); err != nil {
			return err
		}
		if err := os.Chmod(filepath.Dir(p.path), p.dirPerm); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(p.path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 0 {
		buf := make([]byte, 1)
		if _, err := f.ReadAt(buf, info.Size()-1); err != nil {
			return err
		}
		if buf[0] != '\n' {
			if _, err := f.WriteString("\n"); err != nil {
				return err
			}
		}
	}
	for _, line := range p.lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			return err
		}
	}
	return nil
}
