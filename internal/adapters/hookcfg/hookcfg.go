// Package hookcfg installs and removes ccsync auto-sync hooks in Claude Code's
// settings.json. It edits the JSON as a generic map so unrelated settings and
// other hooks are preserved untouched.
package hookcfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// marker identifies commands installed by ccsync, so removal is precise.
const marker = "ccsync"

// events maps the Claude Code hook event to the ccsync command run for it.
func events(exe string) map[string]string {
	return map[string]string{
		"SessionStart": exe + " pull",
		"SessionEnd":   exe + " push",
	}
}

func settingsPath(claudeDir string) string {
	return filepath.Join(claudeDir, "settings.json")
}

// Install adds (idempotently) the ccsync hooks to settings.json, using exe as the
// command path.
func Install(claudeDir, exe string) error {
	data, err := load(claudeDir)
	if err != nil {
		return err
	}
	hooks := asMap(data["hooks"])
	for event, command := range events(exe) {
		groups := asSlice(hooks[event])
		if !containsMarker(groups) {
			groups = append(groups, map[string]any{
				"hooks": []any{
					map[string]any{"type": "command", "command": command},
				},
			})
		}
		hooks[event] = groups
	}
	data["hooks"] = hooks
	return save(claudeDir, data)
}

// Remove strips ccsync's hook groups from settings.json, leaving others intact.
func Remove(claudeDir string) error {
	data, err := load(claudeDir)
	if err != nil {
		return err
	}
	hooks := asMap(data["hooks"])
	for event := range hooks {
		kept := []any{}
		for _, g := range asSlice(hooks[event]) {
			if !groupHasMarker(g) {
				kept = append(kept, g)
			}
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	if len(hooks) == 0 {
		delete(data, "hooks")
	} else {
		data["hooks"] = hooks
	}
	return save(claudeDir, data)
}

func load(claudeDir string) (map[string]any, error) {
	raw, err := os.ReadFile(settingsPath(claudeDir))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	data := map[string]any{}
	if len(raw) == 0 {
		return data, nil
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func save(claudeDir string, data map[string]any) error {
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath(claudeDir), append(out, '\n'), 0o644)
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func containsMarker(groups []any) bool {
	for _, g := range groups {
		if groupHasMarker(g) {
			return true
		}
	}
	return false
}

func groupHasMarker(g any) bool {
	gm := asMap(g)
	for _, h := range asSlice(gm["hooks"]) {
		hm := asMap(h)
		if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, marker) {
			return true
		}
	}
	return false
}
