package claudefs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/melihemreguler/claude-code-sync/internal/domain"
)

func writeSession(t *testing.T, dir, file, cwd string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"user","cwd":"` + cwd + `","sessionId":"x"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, file), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ReadCwd must return this device's cwd — the one whose encoding matches the
// folder name — not a foreign session's cwd that may sit in the same folder
// after a cross-device sync. (Regression test for an early bug.)
func TestReadCwdPrefersFolderMatch(t *testing.T) {
	claude := t.TempDir()
	localCwd := "/Users/me/dev/github/x"
	foreignCwd := "/Users/other/github/x"
	folder := domain.EncodeCwd(localCwd)
	dir := filepath.Join(claude, "projects", folder)

	// Foreign file sorts first; ReadCwd must still return the local cwd.
	writeSession(t, dir, "a-foreign.jsonl", foreignCwd)
	writeSession(t, dir, "z-local.jsonl", localCwd)

	got, err := New(claude).ReadCwd(folder)
	if err != nil {
		t.Fatal(err)
	}
	if got != localCwd {
		t.Errorf("ReadCwd = %q, want local %q", got, localCwd)
	}
}

func TestListProjects(t *testing.T) {
	claude := t.TempDir()
	writeSession(t, filepath.Join(claude, "projects", "-Users-me-x"), "s.jsonl", "/Users/me/x")
	got, err := New(claude).ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "-Users-me-x" {
		t.Errorf("ListProjects = %v, want [-Users-me-x]", got)
	}
}
