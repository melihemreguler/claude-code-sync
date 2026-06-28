// Package claudefs implements ports.ClaudeStore over the local filesystem.
package claudefs

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/melihemreguler/claude-code-sync/internal/domain"
)

// cwdScanLimit bounds how many lines of a session file we read looking for the
// "cwd" field; it appears in an early record.
const cwdScanLimit = 200

// Store is a filesystem-backed Claude Code session store.
type Store struct {
	claudeDir string
}

// New returns a Store rooted at claudeDir (typically ~/.claude).
func New(claudeDir string) *Store {
	return &Store{claudeDir: claudeDir}
}

func (s *Store) projectsDir() string {
	return filepath.Join(s.claudeDir, "projects")
}

// ProjectPath returns the absolute path to a project folder.
func (s *Store) ProjectPath(folder string) string {
	return filepath.Join(s.projectsDir(), folder)
}

// ListProjects returns the project folder names under projects/.
func (s *Store) ListProjects() ([]string, error) {
	entries, err := os.ReadDir(s.projectsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// ReadCwd returns this device's working directory for a project folder.
//
// A folder may contain sessions synced from other devices that carry a different
// cwd, so we cannot just take the first one. The local cwd is the one whose
// encoding matches the folder name (Claude Code named the folder from it); any
// other value is a foreign session. A non-matching value is only returned as a
// last resort when no local session is present.
func (s *Store) ReadCwd(folder string) (string, error) {
	dir := s.ProjectPath(folder)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var fallback string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		cwd, err := scanCwd(filepath.Join(dir, e.Name()))
		if err != nil {
			return "", err
		}
		if cwd == "" {
			continue
		}
		if domain.EncodeCwd(cwd) == folder {
			return cwd, nil
		}
		if fallback == "" {
			fallback = cwd
		}
	}
	return fallback, nil
}

func scanCwd(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for i := 0; i < cwdScanLimit && sc.Scan(); i++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || !strings.Contains(line, "\"cwd\"") {
			continue
		}
		var rec struct {
			Cwd string `json:"cwd"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec.Cwd != "" {
			return rec.Cwd, nil
		}
	}
	return "", sc.Err()
}
