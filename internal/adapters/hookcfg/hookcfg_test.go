package hookcfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallPreservesOtherSettingsAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	// Pre-existing settings with an unrelated key and a foreign hook.
	seed := `{"model":"opus","hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"other-tool run"}]}]}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Install(dir, "/usr/local/bin/ccsync"); err != nil {
		t.Fatal(err)
	}
	if err := Install(dir, "/usr/local/bin/ccsync"); err != nil { // idempotent
		t.Fatal(err)
	}

	data := read(t, dir)
	if data["model"] != "opus" {
		t.Error("unrelated setting was lost")
	}
	starts := groups(t, data, "SessionStart")
	if len(starts) != 2 {
		t.Fatalf("SessionStart should have foreign + one ccsync group, got %d", len(starts))
	}
	if !hasCommand(data, "SessionEnd", "/usr/local/bin/ccsync push") {
		t.Error("SessionEnd ccsync hook missing")
	}

	// Remove strips only ccsync, keeps the foreign hook.
	if err := Remove(dir); err != nil {
		t.Fatal(err)
	}
	data = read(t, dir)
	starts = groups(t, data, "SessionStart")
	if len(starts) != 1 {
		t.Fatalf("after remove, only the foreign hook should remain, got %d", len(starts))
	}
	if _, ok := asMap(data["hooks"])["SessionEnd"]; ok {
		t.Error("SessionEnd should be gone after remove")
	}
}

func read(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func groups(t *testing.T, data map[string]any, event string) []any {
	t.Helper()
	return asSlice(asMap(data["hooks"])[event])
}

func hasCommand(data map[string]any, event, command string) bool {
	for _, g := range asSlice(asMap(data["hooks"])[event]) {
		for _, h := range asSlice(asMap(g)["hooks"]) {
			if cmd, _ := asMap(h)["command"].(string); cmd == command {
				return true
			}
		}
	}
	return false
}
