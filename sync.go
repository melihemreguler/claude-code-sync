package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// projectsSubdir is where Claude Code stores per-project session files, and the
// mirror path inside the data repo.
const projectsSubdir = "projects"

// copyResult tallies what a copy pass touched.
type copyResult struct {
	files    int
	projects int
}

// copyNewer mirrors matching project folders from src/projects to dst/projects,
// copying a file only when it is missing or strictly newer at the source. It
// never deletes anything at the destination — sync is additive and last-writer-
// wins per file, with git union-merge as the backstop for concurrent edits.
func copyNewer(src, dst string, include, exclude []string) (copyResult, error) {
	var res copyResult
	srcProjects := filepath.Join(src, projectsSubdir)
	entries, err := os.ReadDir(srcProjects)
	if err != nil {
		if os.IsNotExist(err) {
			return res, nil
		}
		return res, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !matchFilter(name, include, exclude) {
			continue
		}
		n, err := copyTreeNewer(filepath.Join(srcProjects, name), filepath.Join(dst, projectsSubdir, name))
		if err != nil {
			return res, err
		}
		if n > 0 {
			res.projects++
			res.files += n
		}
	}
	return res, nil
}

func copyTreeNewer(srcDir, dstDir string) (int, error) {
	count := 0
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		di, err := os.Stat(target)
		if err == nil && !info.ModTime().After(di.ModTime()) {
			return nil // destination is same age or newer — skip
		}
		if err := copyFile(path, target); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
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
	tmp := dst + ".ccsync.tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	if si, err := os.Stat(src); err == nil {
		_ = os.Chtimes(dst, si.ModTime(), si.ModTime())
	}
	return nil
}

// ensureRepo makes sure the data repo is cloned locally at cfg.WorkDir.
func ensureRepo(cfg *Config) error {
	if isGitRepo(cfg.WorkDir) {
		return nil
	}
	if cfg.RepoURL == "" {
		return fmt.Errorf("no repoUrl configured")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.WorkDir), 0o755); err != nil {
		return err
	}
	fmt.Printf("Cloning %s …\n", cfg.RepoURL)
	if err := gitStream(filepath.Dir(cfg.WorkDir), "clone", cfg.RepoURL, cfg.WorkDir); err != nil {
		return fmt.Errorf("clone failed: %w", err)
	}
	return nil
}

// pull fetches remote changes and applies newer sessions into the Claude dir.
func pull(cfg *Config) (copyResult, error) {
	if err := ensureRepo(cfg); err != nil {
		return copyResult{}, err
	}
	// A brand-new data repo has no branches yet — nothing to pull.
	heads, _ := git(cfg.WorkDir, "ls-remote", "--heads", "origin")
	if strings.TrimSpace(heads) == "" {
		return copyResult{}, nil
	}
	if err := gitStream(cfg.WorkDir, "pull", "--rebase", "--autostash"); err != nil {
		return copyResult{}, fmt.Errorf("pull failed: %w", err)
	}
	return copyNewer(cfg.WorkDir, cfg.ClaudeDir, cfg.Include, cfg.Exclude)
}

// doPush pushes the current branch, setting upstream on the first push.
func doPush(workDir string) error {
	if _, err := git(workDir, "rev-parse", "--abbrev-ref", "@{u}"); err == nil {
		return gitStream(workDir, "push")
	}
	return gitStream(workDir, "push", "-u", "origin", "HEAD")
}

// push copies newer local sessions into the repo, records this device, commits
// and pushes. It retries once after a rebase if the remote moved underneath us.
func push(cfg *Config) (copyResult, error) {
	if err := ensureRepo(cfg); err != nil {
		return copyResult{}, err
	}
	res, err := copyNewer(cfg.ClaudeDir, cfg.WorkDir, cfg.Include, cfg.Exclude)
	if err != nil {
		return res, err
	}

	reg, err := loadRegistry(cfg.WorkDir)
	if err != nil {
		return res, err
	}
	reg.upsert(cfg.Device, platform())
	if err := reg.save(cfg.WorkDir); err != nil {
		return res, err
	}

	changed, err := hasChanges(cfg.WorkDir)
	if err != nil {
		return res, err
	}
	if !changed {
		return res, nil
	}

	if _, err := git(cfg.WorkDir, "add", "-A"); err != nil {
		return res, err
	}
	msg := fmt.Sprintf("sync: %d file(s) from %s", res.files, cfg.Device)
	if _, err := git(cfg.WorkDir, "commit", "-m", msg); err != nil {
		return res, err
	}
	if err := doPush(cfg.WorkDir); err != nil {
		// Remote likely moved — rebase on top and retry once.
		if e := gitStream(cfg.WorkDir, "pull", "--rebase", "--autostash"); e != nil {
			return res, fmt.Errorf("push rejected and rebase failed: %w", e)
		}
		if err := doPush(cfg.WorkDir); err != nil {
			return res, fmt.Errorf("push failed after rebase: %w", err)
		}
	}
	return res, nil
}
