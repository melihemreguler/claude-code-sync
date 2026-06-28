// Package registry manages the synced device roster (the "control panel"),
// stored as devices.json at the root of the data repo so every machine sees the
// same list.
package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// FileName is the registry file at the data repo root.
const FileName = "devices.json"

// Registry is the set of devices participating in the sync chain.
type Registry struct {
	Devices []Device `json:"devices"`
}

// Device is one machine in the chain.
type Device struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
	AddedAt  string `json:"addedAt"`
	LastSync string `json:"lastSync"`
}

func path(workDir string) string {
	return filepath.Join(workDir, FileName)
}

// Load reads devices.json, returning an empty registry if it does not exist yet.
func Load(workDir string) (*Registry, error) {
	data, err := os.ReadFile(path(workDir))
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{}, nil
		}
		return nil, err
	}
	var r Registry
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", FileName, err)
	}
	return &r, nil
}

// Save writes devices.json with a stable (name-sorted) ordering.
func (r *Registry) Save(workDir string) error {
	sort.Slice(r.Devices, func(i, j int) bool { return r.Devices[i].Name < r.Devices[j].Name })
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(workDir), append(data, '\n'), 0o644)
}

// Find returns the device with the given name, or nil.
func (r *Registry) Find(name string) *Device {
	for i := range r.Devices {
		if r.Devices[i].Name == name {
			return &r.Devices[i]
		}
	}
	return nil
}

// Upsert adds the device if missing and refreshes its LastSync timestamp.
func (r *Registry) Upsert(name, platform string) {
	now := time.Now().UTC().Format(time.RFC3339)
	if d := r.Find(name); d != nil {
		d.LastSync = now
		if d.Platform == "" {
			d.Platform = platform
		}
		return
	}
	r.Devices = append(r.Devices, Device{
		Name:     name,
		Platform: platform,
		AddedAt:  now,
		LastSync: now,
	})
}

// Remove drops a device from the chain, reporting whether it existed.
func (r *Registry) Remove(name string) bool {
	for i := range r.Devices {
		if r.Devices[i].Name == name {
			r.Devices = append(r.Devices[:i], r.Devices[i+1:]...)
			return true
		}
	}
	return false
}
