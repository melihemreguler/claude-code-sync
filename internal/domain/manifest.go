package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"
)

// Manifest is the synced index of the sync chain. It records the devices and, for
// every logical project, that project's session-object location plus the local
// folder name each device uses for it (the path-translation table).
//
// In later phases the manifest is stored encrypted, because project paths are
// sensitive metadata.
type Manifest struct {
	Devices  []DeviceEntry           `json:"devices"`
	Projects map[string]ProjectEntry `json:"projects"` // keyed by string(CanonicalKey)
}

// DeviceEntry is one machine in the chain, including the project roots it has
// chosen to sync or exclude (surfaced by `ccsync device list`).
type DeviceEntry struct {
	Name     string   `json:"name"`
	Platform string   `json:"platform"`
	AddedAt  string   `json:"addedAt"`
	LastSync string   `json:"lastSync"`
	Include  []string `json:"include"`
	Exclude  []string `json:"exclude"`
}

// ProjectEntry maps a logical project to each device's local folder name for it.
type ProjectEntry struct {
	Display string            `json:"display"`
	Folders map[string]string `json:"folders"` // device name -> local folder name
}

// KeyHash returns a filesystem-safe directory name for a canonical key.
func KeyHash(key CanonicalKey) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// NewManifest returns an initialized, empty manifest.
func NewManifest() *Manifest {
	return &Manifest{Projects: map[string]ProjectEntry{}}
}

// FindDevice returns the named device entry, or nil.
func (m *Manifest) FindDevice(name string) *DeviceEntry {
	for i := range m.Devices {
		if m.Devices[i].Name == name {
			return &m.Devices[i]
		}
	}
	return nil
}

// UpsertDevice adds or updates a device, refreshing its LastSync and selections.
func (m *Manifest) UpsertDevice(name, platform string, include, exclude []string) {
	now := time.Now().UTC().Format(time.RFC3339)
	if d := m.FindDevice(name); d != nil {
		d.LastSync = now
		d.Platform = platform
		d.Include = include
		d.Exclude = exclude
		return
	}
	m.Devices = append(m.Devices, DeviceEntry{
		Name: name, Platform: platform, AddedAt: now, LastSync: now,
		Include: include, Exclude: exclude,
	})
}

// RemoveDevice drops a device and its folder mappings, reporting whether it
// existed.
func (m *Manifest) RemoveDevice(name string) bool {
	found := false
	for i := range m.Devices {
		if m.Devices[i].Name == name {
			m.Devices = append(m.Devices[:i], m.Devices[i+1:]...)
			found = true
			break
		}
	}
	for key, entry := range m.Projects {
		if _, ok := entry.Folders[name]; ok {
			delete(entry.Folders, name)
			m.Projects[key] = entry
		}
	}
	return found
}

// RecordProject notes that, on device, the given logical project lives in folder.
func (m *Manifest) RecordProject(key CanonicalKey, display, device, folder string) {
	if m.Projects == nil {
		m.Projects = map[string]ProjectEntry{}
	}
	entry, ok := m.Projects[string(key)]
	if !ok {
		entry = ProjectEntry{Folders: map[string]string{}}
	}
	if entry.Folders == nil {
		entry.Folders = map[string]string{}
	}
	if display != "" {
		entry.Display = display
	}
	entry.Folders[device] = folder
	m.Projects[string(key)] = entry
}

// SortedDevices returns devices ordered by name for stable output.
func (m *Manifest) SortedDevices() []DeviceEntry {
	out := append([]DeviceEntry(nil), m.Devices...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
