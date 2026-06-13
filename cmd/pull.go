package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/coffee-code-io/coffeeenv/internal/chart"
)

var pullName string

var pullCmd = &cobra.Command{
	Use:   "pull <source>",
	Short: "Fetch a CUE chart into ~/.coffeeenv/charts",
	Long: `Fetch a chart from a local directory or a git+https URL into
~/.coffeeenv/charts/<name>, replacing any previous contents.

Examples:
  coffeeenv pull ./examples/claude-basic
  coffeeenv pull git+https://github.com/you/envs.git#main:claude --name claude`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source := args[0]
		name := pullName
		if name == "" {
			name = chartNameFromSource(source)
		}
		c, err := chart.Open(name)
		if err != nil {
			return err
		}

		ref, commit, digest, err := c.Pull(cmd.Context(), source)
		if err != nil {
			return err
		}
		if err := c.WriteLock(chart.LockInfo{
			Source:   source,
			Ref:      ref,
			Commit:   commit,
			Digest:   digest,
			PulledAt: time.Now().UTC().Format(time.RFC3339),
		}); err != nil {
			return err
		}

		fmt.Printf("Pulled %s into chart %q (%s)\n", source, name, c.Dir)
		if commit != "" {
			fmt.Printf("  commit %s\n", commit)
		}

		// Recursively pull the chart's declared dependencies (BFS, deduped).
		if n, err := pullDeps(cmd.Context(), c); err != nil {
			return err
		} else if n > 0 {
			fmt.Printf("  pulled %d dependenc(ies)\n", n)
		}

		fmt.Printf("Next: coffeeenv plan %s\n", name)
		return nil
	},
}

// pullDeps fetches the transitive `dependencies` of root, breadth-first,
// deduping by source. Returns how many new charts were pulled.
func pullDeps(ctx context.Context, root chart.Chart) (int, error) {
	seen := map[string]bool{}
	queue := []chart.Chart{root}
	pulled := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		m, ok, err := cur.ReadManifest()
		if err != nil {
			return pulled, err
		}
		if !ok {
			continue
		}
		for _, src := range m.Dependencies {
			if seen[src] {
				continue
			}
			seen[src] = true
			dc, err := chart.Open(chartNameFromSource(src))
			if err != nil {
				return pulled, err
			}
			if !dc.Exists() {
				ref, commit, digest, err := dc.Pull(ctx, src)
				if err != nil {
					return pulled, fmt.Errorf("dependency %q: %w", src, err)
				}
				if err := dc.WriteLock(chart.LockInfo{Source: src, Ref: ref, Commit: commit, Digest: digest,
					PulledAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
					return pulled, err
				}
				pulled++
			}
			queue = append(queue, dc)
		}
	}
	return pulled, nil
}

func init() {
	pullCmd.Flags().StringVar(&pullName, "name", "", "chart name (default: basename of source)")
}

// chartNameFromSource derives a slug from a local path or git URL+subpath.
func chartNameFromSource(source string) string {
	s := source
	if i := strings.Index(s, "#"); i >= 0 {
		frag := s[i+1:]
		s = s[:i]
		if j := strings.Index(frag, ":"); j >= 0 {
			s = frag[j+1:] // prefer the subpath
		}
	}
	s = strings.TrimSuffix(s, "/")
	base := filepath.Base(s)
	base = strings.TrimSuffix(base, ".git")
	if base == "" || base == "." || base == "/" {
		return "default"
	}
	return base
}
