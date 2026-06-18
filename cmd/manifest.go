package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/coffee-code-io/coffeeenv/internal/chart"
	"github.com/coffee-code-io/coffeeenv/internal/sys"
)

// globalManifestPath is ~/.coffeeenv/manifest.json (honoring $COFFEEENV_ROOT) —
// the composition for the global (non-venv) setup.
func globalManifestPath() (string, error) {
	root := os.Getenv("COFFEEENV_ROOT")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".coffeeenv")
	}
	return filepath.Join(root, "manifest.json"), nil
}

func readGlobalManifest() (chart.Manifest, error) {
	p, err := globalManifestPath()
	if err != nil {
		return chart.Manifest{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return chart.Manifest{Module: "coffeeenv.dev/global", Type: "executable"}, nil
		}
		return chart.Manifest{}, err
	}
	var m chart.Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return chart.Manifest{}, err
	}
	return m, nil
}

func writeGlobalManifest(m chart.Manifest) error {
	p, err := globalManifestPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return sys.WriteFileAtomic(p, append(b, '\n'), 0o644)
}

// depsIndex maps module path -> chart dir for every pulled CUE chart, so the
// composition can `import` any of them. Skill charts are excluded — they ship no
// CUE and are added to the composition as file-backed skills, not imports.
func depsIndex() (map[string]string, error) {
	idx, err := chart.Index()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(idx))
	for module, c := range idx {
		m, ok, err := c.ReadManifest()
		if err != nil {
			return nil, err
		}
		if ok && m.Type == "skill" {
			continue
		}
		out[module] = c.Dir
	}
	return out, nil
}

// skillDirsFor resolves a manifest's skill names to their pulled directories.
func skillDirsFor(names []string) (map[string]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	idx, err := chart.SkillsIndex()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(names))
	for _, n := range names {
		dir, ok := idx[n]
		if !ok {
			return nil, fmt.Errorf("skill %q is not pulled", n)
		}
		out[n] = dir
	}
	return out, nil
}

func appendDedup(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

// mergeValues returns a∪b with b winning on conflicts.
func mergeValues(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
