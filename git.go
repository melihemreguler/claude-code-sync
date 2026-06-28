package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// git runs a git command inside dir and returns trimmed stdout.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// gitStream runs git with output attached to the terminal (for clone/push progress).
func gitStream(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func isGitRepo(dir string) bool {
	_, err := git(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

// hasChanges reports whether the working tree has staged or unstaged changes.
func hasChanges(dir string) (bool, error) {
	out, err := git(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}
