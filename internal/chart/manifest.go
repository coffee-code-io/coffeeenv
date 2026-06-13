package chart

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/coffee-code-io/coffeeenv/internal/sys"
)

// Manifest is a chart's author-provided metadata (manifest.json at the chart
// root). It is distinct from the provenance LockInfo. A composition target (a
// venv or the global setup) is itself a chart described by such a manifest, with
// `execs` and `values` accumulated as charts are applied.
type Manifest struct {
	Module       string            `json:"module"`                 // CUE module/package root (import path)
	Version      string            `json:"version,omitempty"`
	Doc          string            `json:"doc,omitempty"`
	Type         string            `json:"type"`                   // "executable" | "library"
	Dependencies []string          `json:"dependencies,omitempty"` // sources to pull (any transport)
	Execs        []string          `json:"execs,omitempty"`        // module paths of executable deps to run
	Values       map[string]string `json:"values,omitempty"`       // resolved inputs (flat path -> value)
}

// ManifestPath is the path to the chart's manifest.json.
func (c Chart) ManifestPath() string { return filepath.Join(c.Dir, "manifest.json") }

// ReadManifest loads the chart's manifest.json, if present.
func (c Chart) ReadManifest() (Manifest, bool, error) {
	b, err := os.ReadFile(c.ManifestPath())
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, false, nil
		}
		return Manifest{}, false, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, false, err
	}
	return m, true, nil
}

// WriteManifest persists the chart's manifest.json.
func (c Chart) WriteManifest(m Manifest) error {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return sys.WriteFileAtomic(c.ManifestPath(), append(b, '\n'), 0o644)
}

// Index maps every pulled chart's module path to its Chart. Charts without a
// manifest (or module) are keyed by name as a fallback so bare pulls still work.
func Index() (map[string]Chart, error) {
	names, err := List()
	if err != nil {
		return nil, err
	}
	out := map[string]Chart{}
	for _, name := range names {
		c, err := Open(name)
		if err != nil {
			return nil, err
		}
		m, ok, err := c.ReadManifest()
		if err != nil {
			return nil, err
		}
		if ok && m.Module != "" {
			out[m.Module] = c
		} else {
			out[name] = c
		}
	}
	return out, nil
}
