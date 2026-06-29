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

// ProjectEntry maps a logical project to each device's local folder name for it,
// and tracks the encrypted session objects belonging to the project.
type ProjectEntry struct {
	Display string                `json:"display"`
	Folders map[string]string     `json:"folders"` // device name -> local folder name
	Objects map[string]ObjectMeta `json:"objects"` // relpath (slash-separated) -> meta
}

// ObjectMeta is the authoritative state of one session file. Because age
// ciphertext is non-deterministic (the same plaintext encrypts differently every
// time), change detection uses the plaintext Hash, and newness uses MTime — both
// kept here inside the encrypted manifest rather than inferred from ciphertext or
// from git, which does not preserve modification times.
type ObjectMeta struct {
	Hash  string `json:"hash"`  // sha256 hex of the plaintext
	MTime int64  `json:"mtime"` // source modification time, unix nanoseconds
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

// RecordProject notes that, on device, the given logical project lives in folder.
func (m *Manifest) RecordProject(key CanonicalKey, display, device, folder string) {
	if m.Projects == nil {
		m.Projects = map[string]ProjectEntry{}
	}
	entry := m.project(key)
	if display != "" {
		entry.Display = display
	}
	entry.Folders[device] = folder
	m.Projects[string(key)] = entry
}

// SetObject records the metadata for one session object under a project.
func (m *Manifest) SetObject(key CanonicalKey, relpath string, meta ObjectMeta) {
	entry := m.project(key)
	entry.Objects[relpath] = meta
	m.Projects[string(key)] = entry
}

// project returns the entry for key with its maps initialized.
func (m *Manifest) project(key CanonicalKey) ProjectEntry {
	if m.Projects == nil {
		m.Projects = map[string]ProjectEntry{}
	}
	entry := m.Projects[string(key)]
	if entry.Folders == nil {
		entry.Folders = map[string]string{}
	}
	if entry.Objects == nil {
		entry.Objects = map[string]ObjectMeta{}
	}
	return entry
}

// Merge folds other into m: union of devices (latest LastSync wins), project
// folder mappings, and per-object metadata (latest MTime wins). It is how the
// per-device manifest shards combine into one view.
func (m *Manifest) Merge(other *Manifest) {
	for _, d := range other.Devices {
		if cur := m.FindDevice(d.Name); cur != nil {
			if d.LastSync > cur.LastSync {
				*cur = d
			}
		} else {
			m.Devices = append(m.Devices, d)
		}
	}
	for key, oe := range other.Projects {
		e := m.project(CanonicalKey(key))
		if oe.Display != "" {
			e.Display = oe.Display
		}
		for dev, folder := range oe.Folders {
			e.Folders[dev] = folder
		}
		for rel, om := range oe.Objects {
			if cur, ok := e.Objects[rel]; !ok || om.MTime > cur.MTime {
				e.Objects[rel] = om
			}
		}
		m.Projects[key] = e
	}
}

// SortedDevices returns devices ordered by name for stable output.
func (m *Manifest) SortedDevices() []DeviceEntry {
	out := append([]DeviceEntry(nil), m.Devices...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
