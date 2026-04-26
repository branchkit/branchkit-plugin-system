package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	shared "github.com/branchkit/plugin-sdk-go"
	toolkit "github.com/branchkit/plugin-toolkit-go"
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
	appsMu sync.Mutex
	apps   []AppEntry
)

// initApps loads installed apps, merges curated aliases, and pushes the
// flat collection. User overrides (aliases, disabled) are handled by the
// platform collection override system.
func initApps(p *shared.Plugin) {
	// 1. Scan installed apps via native method
	scanned := scanInstalledApps(p)

	// 2. Load curated core aliases (shipped alongside plugin binary)
	coreApps := loadAppsFile(filepath.Join(toolkit.PluginDir(), "core_apps.json"))
	scanned = mergeAliases(scanned, coreApps)

	appsMu.Lock()
	apps = scanned
	appsMu.Unlock()

	pushAppsCollection(p)

	// Sync enabled/disabled state from platform overrides.
	// If a user previously disabled an app, the override has its aliases
	// in the 'removed' map. Mark those apps as disabled in the internal model
	// so the HUD and settings UI reflect the persisted state.
	syncDisabledFromOverrides(p)
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
			scanned = append(scanned, ca)
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

// syncDisabledFromOverrides reads the platform's collection_overrides for the
// apps collection and marks apps as disabled if ALL their aliases are removed.
func syncDisabledFromOverrides(p *shared.Plugin) {
	// Read the current named_lists["apps"] — this has overrides already applied.
	// Compare with our internal list to find apps whose aliases are all removed.
	var resp struct {
		Data map[string]string `json:"data"`
	}
	if err := p.Call("collection.get", map[string]string{"name": "apps"}, &resp); err != nil {
		return
	}

	appsMu.Lock()
	defer appsMu.Unlock()

	activeAliases := make(map[string]bool, len(resp.Data))
	for spoken := range resp.Data {
		activeAliases[spoken] = true
	}

	for i := range apps {
		allRemoved := true
		for _, alias := range apps[i].Aliases {
			if activeAliases[strings.ToLower(alias)] {
				allRemoved = false
				break
			}
		}
		if allRemoved && len(apps[i].Aliases) > 0 {
			apps[i].Enabled = false
		}
	}
}

// pushAppsCollection derives the flat collection entries and pushes them.
// Pushes ALL apps (platform overrides handle disabling via removed entries).
func pushAppsCollection(p *shared.Plugin) {
	type entry struct {
		Spoken   string `json:"spoken"`
		BundleID string `json:"bundle_id"`
	}

	appsMu.Lock()
	var flat []entry
	for _, app := range apps {
		for _, alias := range app.Aliases {
			flat = append(flat, entry{
				Spoken:   strings.ToLower(alias),
				BundleID: app.BundleID,
			})
		}
	}
	appsMu.Unlock()

	if err := toolkit.PushCollection(p, "apps", flat, toolkit.WithLabel("Apps")); err != nil {
		shared.Logf("system", "collection.push apps: %v", err)
	}
}

// --- Mutations (route through platform collection.override) ---

func toggleApp(p *shared.Plugin, bundleID string) {
	appsMu.Lock()
	var aliases []string
	var nowEnabled bool
	for i := range apps {
		if apps[i].BundleID == bundleID {
			apps[i].Enabled = !apps[i].Enabled
			nowEnabled = apps[i].Enabled
			aliases = append([]string{}, apps[i].Aliases...)
			break
		}
	}
	appsMu.Unlock()

	// Add/remove each alias via platform override
	for _, alias := range aliases {
		spoken := strings.ToLower(alias)
		if nowEnabled {
			// Re-enable: remove the override so plugin data takes effect
			_ = p.Call("collection.override", map[string]any{
				"collection": "apps", "action": "add",
				"key": spoken, "value": bundleID,
			}, nil)
		} else {
			// Disable: suppress each alias
			_ = p.Call("collection.override", map[string]any{
				"collection": "apps", "action": "remove", "key": spoken,
			}, nil)
		}
	}
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

	// Add via platform override
	_ = p.Call("collection.override", map[string]any{
		"collection": "apps", "action": "add",
		"key": alias, "value": bundleID,
	}, nil)
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

	// Remove via platform override
	_ = p.Call("collection.override", map[string]any{
		"collection": "apps", "action": "remove", "key": alias,
	}, nil)
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
