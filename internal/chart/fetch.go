package chart

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/coffee-code-io/coffeeenv/internal/sys"
)

// Pull fetches the environment definition at source into the chart directory,
// wiping any previous contents. Supported transports are dispatched by scheme:
//
//	git+https:// | git+ssh:// | git@… | *.git   -> git clone (with #ref:subpath)
//	oci://<ref>                                  -> `oras pull` (shell-out)
//	local://<path> | <existing local dir>        -> copy a local directory
//
// It returns ref/commit (git) and digest (oci) for the lock file.
func (c Chart) Pull(ctx context.Context, source string) (ref, commit, digest string, err error) {
	var srcDir string
	cleanup := func() {}

	switch {
	case isOCISource(source):
		srcDir, digest, cleanup, err = fetchOCI(ctx, source)
	case isGitSource(source):
		srcDir, ref, commit, cleanup, err = fetchGit(ctx, source)
	default:
		srcDir, err = localDir(source)
	}
	if err != nil {
		return "", "", "", err
	}
	defer cleanup()

	if !hasCueFile(srcDir) && !hasManifest(srcDir) {
		return "", "", "", fmt.Errorf("source %q has no .cue files or manifest.json", source)
	}

	if err := os.RemoveAll(c.Dir); err != nil {
		return "", "", "", err
	}
	if err := copyTree(srcDir, c.Dir); err != nil {
		return "", "", "", err
	}
	if err := c.ensureModule(); err != nil {
		return "", "", "", err
	}
	return ref, commit, digest, nil
}

// localDir resolves a local source: a bare path or a local://<path> URL.
// local:///abs is absolute; local://./rel and local://rel resolve against cwd.
func localDir(source string) (string, error) {
	p := strings.TrimPrefix(source, "local://")
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if fi, statErr := os.Stat(abs); statErr != nil || !fi.IsDir() {
		return "", fmt.Errorf("source %q is not a directory", source)
	}
	return abs, nil
}

func isOCISource(s string) bool { return strings.HasPrefix(s, "oci://") }

// fetchOCI pulls an OCI artifact into a temp dir by shelling out to `oras`.
func fetchOCI(ctx context.Context, source string) (dir, digest string, cleanup func(), err error) {
	ref := strings.TrimPrefix(source, "oci://")
	if !sys.Look("oras") {
		return "", "", func() {}, fmt.Errorf("oci:// requires `oras` on PATH (https://oras.land); cannot pull %q", source)
	}
	tmp, err := os.MkdirTemp("", "coffeeenv-oci-*")
	if err != nil {
		return "", "", func() {}, err
	}
	cleanup = func() { os.RemoveAll(tmp) }
	if pullErr := sys.Stream(ctx, "oras", "pull", ref, "--output", tmp); pullErr != nil {
		cleanup()
		return "", "", func() {}, fmt.Errorf("oras pull: %w", pullErr)
	}
	// Best-effort digest for the lock file.
	if res, rerr := sys.Run(ctx, "oras", "resolve", ref); rerr == nil {
		digest = strings.TrimSpace(res.Stdout)
	}
	return tmp, digest, cleanup, nil
}

// ensureModule writes cue.mod/module.cue if the pulled source didn't ship one.
func (c Chart) ensureModule() error {
	if _, err := os.Stat(c.CueModule()); err == nil {
		return nil
	}
	const mod = "module: \"coffeeenv.dev/user\"\nlanguage: version: \"v0.9.0\"\n"
	return sys.WriteFileAtomic(c.CueModule(), []byte(mod), 0o644)
}

func isGitSource(s string) bool {
	return strings.HasPrefix(s, "git+") ||
		strings.HasPrefix(s, "git@") ||
		strings.HasSuffix(strings.SplitN(s, "#", 2)[0], ".git")
}

// fetchGit clones a git+https URL of the form
//
//	git+https://host/user/repo.git#ref:subpath
//
// (ref and subpath optional) into a temp dir and returns the effective source
// directory (the subpath if given), the ref, and the resolved commit.
func fetchGit(ctx context.Context, source string) (dir, ref, commit string, cleanup func(), err error) {
	url := strings.TrimPrefix(source, "git+")
	var subpath string
	if i := strings.Index(url, "#"); i >= 0 {
		frag := url[i+1:]
		url = url[:i]
		if j := strings.Index(frag, ":"); j >= 0 {
			ref, subpath = frag[:j], frag[j+1:]
		} else {
			ref = frag
		}
	}

	tmp, err := os.MkdirTemp("", "coffeeenv-pull-*")
	if err != nil {
		return "", "", "", func() {}, err
	}
	cleanup = func() { os.RemoveAll(tmp) }

	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, url, tmp)
	if cloneErr := sys.Stream(ctx, "git", args...); cloneErr != nil {
		cleanup()
		return "", "", "", func() {}, fmt.Errorf("git clone: %w", cloneErr)
	}

	if res, revErr := sys.Run(ctx, "git", "-C", tmp, "rev-parse", "HEAD"); revErr == nil {
		commit = strings.TrimSpace(res.Stdout)
	}

	dir = tmp
	if subpath != "" {
		dir = filepath.Join(tmp, subpath)
		if fi, statErr := os.Stat(dir); statErr != nil || !fi.IsDir() {
			cleanup()
			return "", "", "", func() {}, fmt.Errorf("subpath %q not found in repo", subpath)
		}
	}
	return dir, ref, commit, cleanup, nil
}

// hasCueFile reports whether dir contains at least one .cue file (recursively,
// skipping the excluded dirs).
func hasCueFile(dir string) bool {
	found := false
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && excluded(d.Name()) && p != dir {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".cue") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func excluded(name string) bool { return name == ".git" || name == "node_modules" }

// hasManifest reports whether dir holds a manifest.json (an exec-only meta-chart
// may carry no .cue files).
func hasManifest(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "manifest.json"))
	return err == nil
}

// copyTree recursively copies src into dst, preserving file modes and skipping
// excluded directories.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if excluded(d.Name()) && p != src {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		return copyFile(p, filepath.Join(dst, rel))
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
