package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Registry is the synced "control panel" of devices participating in the chain.
// It lives at <repo>/devices.json so every machine sees the same list.
type Registry struct {
	Devices []Device `json:"devices"`
}

type Device struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
	AddedAt  string `json:"addedAt"`
	LastSync string `json:"lastSync"`
}

func registryPath(workDir string) string {
	return filepath.Join(workDir, "devices.json")
}

func loadRegistry(workDir string) (*Registry, error) {
	data, err := os.ReadFile(registryPath(workDir))
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{}, nil
		}
		return nil, err
	}
	var r Registry
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing devices.json: %w", err)
	}
	return &r, nil
}

func (r *Registry) save(workDir string) error {
	sort.Slice(r.Devices, func(i, j int) bool { return r.Devices[i].Name < r.Devices[j].Name })
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(registryPath(workDir), append(data, '\n'), 0o644)
}

func (r *Registry) find(name string) *Device {
	for i := range r.Devices {
		if r.Devices[i].Name == name {
			return &r.Devices[i]
		}
	}
	return nil
}

// upsert adds the device if missing and refreshes its lastSync timestamp.
func (r *Registry) upsert(name, plat string) {
	now := time.Now().UTC().Format(time.RFC3339)
	if d := r.find(name); d != nil {
		d.LastSync = now
		if d.Platform == "" {
			d.Platform = plat
		}
		return
	}
	r.Devices = append(r.Devices, Device{
		Name:     name,
		Platform: plat,
		AddedAt:  now,
		LastSync: now,
	})
}

func (r *Registry) remove(name string) bool {
	for i := range r.Devices {
		if r.Devices[i].Name == name {
			r.Devices = append(r.Devices[:i], r.Devices[i+1:]...)
			return true
		}
	}
	return false
}
