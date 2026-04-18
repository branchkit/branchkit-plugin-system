package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	shared "github.com/branchkit/plugin-sdk-go"
)

// AppEntry is the internal rich model for a known application.
type AppEntry struct {
	Name     string   `json:"name"`
	BundleID string   `json:"bundle_id"`
	Aliases  []string `json:"aliases"`
	Enabled  bool     `json:"enabled"`
}

// installedApp matches the native.installed_apps response shape.
type installedApp struct {
	Name     string `json:"name"`
	BundleID string `json:"bundle_id"`
}

var (
	appsMu       sync.Mutex
	apps         []AppEntry
	appsDataPath string
)

// initApps loads installed apps, merges curated aliases and user overrides,
// and pushes the flat collection to the matching engine.
func initApps(p *shared.Plugin) {
	dir := os.Getenv("BRANCHKIT_PLUGIN_DIR")
	if dir == "" {
		dir = "."
	}
	appsDataPath = filepath.Join(dir, "user_apps.json")

	// 1. Scan installed apps via native method
	scanned := scanInstalledApps(p)

	// 2. Load curated core aliases (shipped alongside plugin binary)
	coreApps := loadAppsFile(filepath.Join(dir, "core_apps.json"))
	scanned = mergeAliases(scanned, coreApps)

	// 3. Apply user overrides
	userApps := loadAppsFile(appsDataPath)
	scanned = applyOverrides(scanned, userApps)

	appsMu.Lock()
	apps = scanned
	appsMu.Unlock()

	pushAppsCollection(p)
}

// scanInstalledApps calls native.installed_apps and returns AppEntry slice.
func scanInstalledApps(p *shared.Plugin) []AppEntry {
	var resp struct {
		Apps []installedApp `json:"apps"`
	}
	if err := p.Call("native.installed_apps", struct{}{}, &resp); err != nil {
		shared.Logf("system", "installed_apps: %v", err)
		return nil
	}
	entries := make([]AppEntry, 0, len(resp.Apps))
	for _, app := range resp.Apps {
		alias := strings.ToLower(app.Name)
		entries = append(entries, AppEntry{
			Name:     app.Name,
			BundleID: app.BundleID,
			Aliases:  []string{alias},
			Enabled:  true,
		})
	}
	return entries
}

// mergeAliases merges curated core aliases into the scanned list by bundle_id.
func mergeAliases(scanned []AppEntry, core []AppEntry) []AppEntry {
	idx := make(map[string]int) // lowercase bundle_id → index
	for i, app := range scanned {
		idx[strings.ToLower(app.BundleID)] = i
	}
	for _, ca := range core {
		key := strings.ToLower(ca.BundleID)
		if i, ok := idx[key]; ok {
			for _, alias := range ca.Aliases {
				lower := strings.ToLower(alias)
				if !containsLower(scanned[i].Aliases, lower) {
					scanned[i].Aliases = append(scanned[i].Aliases, lower)
				}
			}
		} else {
			// Core app not installed — add anyway so user can configure it
			scanned = append(scanned, ca)
		}
	}
	return scanned
}

// applyOverrides applies sparse user overrides (aliases, enabled/disabled).
func applyOverrides(scanned []AppEntry, overrides []AppEntry) []AppEntry {
	idx := make(map[string]int)
	for i, app := range scanned {
		idx[strings.ToLower(app.BundleID)] = i
	}
	for _, ov := range overrides {
		key := strings.ToLower(ov.BundleID)
		if i, ok := idx[key]; ok {
			scanned[i].Enabled = ov.Enabled
			for _, alias := range ov.Aliases {
				lower := strings.ToLower(alias)
				if !containsLower(scanned[i].Aliases, lower) {
					scanned[i].Aliases = append(scanned[i].Aliases, lower)
				}
			}
		} else {
			scanned = append(scanned, ov)
		}
	}
	return scanned
}

func containsLower(ss []string, target string) bool {
	for _, s := range ss {
		if strings.ToLower(s) == target {
			return true
		}
	}
	return false
}

// pushAppsCollection derives the flat collection entries and pushes them.
func pushAppsCollection(p *shared.Plugin) {
	type entry struct {
		Spoken   string `json:"spoken"`
		BundleID string `json:"bundle_id"`
	}

	// Build the flat entries under the lock, then release before RPC call
	// to avoid deadlock if the actuator calls back into this plugin.
	appsMu.Lock()
	var flat []entry
	for _, app := range apps {
		if !app.Enabled {
			continue
		}
		for _, alias := range app.Aliases {
			flat = append(flat, entry{
				Spoken:   strings.ToLower(alias),
				BundleID: app.BundleID,
			})
		}
	}
	appsMu.Unlock()

	if err := p.Call("collection.push", map[string]any{
		"name":  "apps",
		"data":  flat,
		"label": "Apps",
	}, nil); err != nil {
		shared.Logf("system", "collection.push apps: %v", err)
	}
}

// --- Mutations ---

func toggleApp(p *shared.Plugin, bundleID string) {
	appsMu.Lock()
	for i := range apps {
		if apps[i].BundleID == bundleID {
			apps[i].Enabled = !apps[i].Enabled
			break
		}
	}
	appsMu.Unlock()
	saveUserApps()
	pushAppsCollection(p)
}

func addAppAlias(p *shared.Plugin, bundleID, alias string) {
	alias = strings.TrimSpace(strings.ToLower(alias))
	if alias == "" {
		return
	}
	appsMu.Lock()
	for i := range apps {
		if apps[i].BundleID == bundleID {
			if !containsLower(apps[i].Aliases, alias) {
				apps[i].Aliases = append(apps[i].Aliases, alias)
			}
			break
		}
	}
	appsMu.Unlock()
	saveUserApps()
	pushAppsCollection(p)
}

func removeAppAlias(p *shared.Plugin, bundleID, alias string) {
	alias = strings.TrimSpace(strings.ToLower(alias))
	appsMu.Lock()
	for i := range apps {
		if apps[i].BundleID == bundleID {
			filtered := apps[i].Aliases[:0]
			for _, a := range apps[i].Aliases {
				if strings.ToLower(a) != alias {
					filtered = append(filtered, a)
				}
			}
			apps[i].Aliases = filtered
			break
		}
	}
	appsMu.Unlock()
	saveUserApps()
	pushAppsCollection(p)
}

// saveUserApps persists user overrides (only apps that differ from defaults).
func saveUserApps() {
	appsMu.Lock()
	// Save all app entries as user state — simple approach for now.
	data, err := json.MarshalIndent(apps, "", "  ")
	appsMu.Unlock()
	if err != nil {
		shared.Logf("system", "save user_apps: %v", err)
		return
	}
	if err := os.WriteFile(appsDataPath, data, 0644); err != nil {
		shared.Logf("system", "write user_apps: %v", err)
	}
}

// getApps returns a snapshot of the current app list.
func getApps() []AppEntry {
	appsMu.Lock()
	defer appsMu.Unlock()
	cp := make([]AppEntry, len(apps))
	copy(cp, apps)
	return cp
}

func loadAppsFile(path string) []AppEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var entries []AppEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		shared.Logf("system", "parse %s: %v", path, err)
		return nil
	}
	return entries
}
