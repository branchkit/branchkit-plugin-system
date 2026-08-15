package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	scanned, scanErr := scanInstalledApps(p)

	// 2. Load curated core aliases (shipped alongside plugin binary)
	coreApps := loadAppsFile(filepath.Join(toolkit.PluginDir(), "core_apps.json"))
	scanned = mergeAliases(scanned, coreApps)

	appsMu.Lock()
	apps = scanned
	appsMu.Unlock()

	pushAppsCollection(p)

	// A failed scan is not a small collection — it is a WRONG one that nothing
	// re-pushes. `apps` is written with a whole-scope replace, so the push above
	// deletes every real app and leaves only the ~24 curated bundles from
	// core_apps.json; "launch <anything else>" is then dead for the rest of the
	// session. There is exactly one call site for pushAppsCollection and no
	// OnReady or change subscription to bring it back, unlike the audio
	// collections (main.go wires both for those).
	//
	// The window is not hypothetical: initApps runs before plugin.Run(), during
	// actuator startup, which is exactly when the native side is most likely to
	// be slow or unresponsive — and the RPC has a 10s timeout.
	//
	// So retry in the background rather than logging and giving up. Bounded,
	// backing off, and it stops at the first success; a permanently broken
	// native side degrades to today's behavior instead of spinning.
	if scanErr != nil {
		go retryAppScan(p)
	}

	// Sync enabled/disabled state from platform overrides.
	// If a user previously disabled an app, the override has its aliases
	// in the 'removed' map. Mark those apps as disabled in the internal model
	// so the HUD and settings UI reflect the persisted state.
	syncDisabledFromOverrides(p)
}

// scanInstalledApps calls native.installed_apps and returns the AppEntry slice.
//
// Returns the error rather than swallowing it, because "the scan failed" and
// "you have no apps installed" are different answers and the caller acts on the
// difference — the first needs a retry, the second does not. Returning a bare
// nil for both is what made a transient failure look like a legitimate result.
func scanInstalledApps(p *shared.Plugin) ([]AppEntry, error) {
	var resp struct {
		Apps []installedApp `json:"apps"`
	}
	if err := p.Call("native.installed_apps", struct{}{}, &resp); err != nil {
		shared.Logf("system", "installed_apps: %v", err)
		return nil, err
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
	return entries, nil
}

// appScanRetryDelays is the backoff for retryAppScan. Bounded on purpose: a
// permanently broken native side should settle into today's behavior (the
// curated core set) rather than retrying forever. Front-loaded because the
// failure this exists for is a slow actuator boot, which resolves in seconds.
var appScanRetryDelays = []time.Duration{
	2 * time.Second,
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	60 * time.Second,
}

// retryAppScan re-runs the installed-apps scan after a failed one and re-pushes
// on the first success. Runs in its own goroutine; stops at the first success or
// when the backoff is exhausted.
func retryAppScan(p *shared.Plugin) {
	for i, delay := range appScanRetryDelays {
		time.Sleep(delay)
		scanned, err := scanInstalledApps(p)
		if err != nil {
			shared.Logf("system", "app scan retry %d/%d: %v", i+1, len(appScanRetryDelays), err)
			continue
		}
		coreApps := loadAppsFile(filepath.Join(toolkit.PluginDir(), "core_apps.json"))
		scanned = mergeAliases(scanned, coreApps)

		appsMu.Lock()
		apps = scanned
		appsMu.Unlock()

		pushAppsCollection(p)
		// Re-apply user overrides: the replace above rewrote the collection, so
		// the disabled set has to be reconciled against it again.
		syncDisabledFromOverrides(p)
		shared.Logf("system", "app scan retry %d/%d succeeded: %d apps",
			i+1, len(appScanRetryDelays), len(scanned))
		return
	}
	shared.Logf("system", "app scan never succeeded — staying on the curated core set")
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
//
// `apps` declares feeds_matching: as_named_entities with key_field "spoken"
// — the lowercased alias is the record id the matcher addresses entries by.
// Multiple aliases per bundle produce multiple records sharing the same
// bundle_id payload field.
//
// Note: the legacy `WithLabel("Apps")` Settings UI section-header
// declaration is dropped here. Section headers will derive from a future
// manifest-level label field; in the interim the section renders as "apps".
func pushAppsCollection(p *shared.Plugin) {
	type entry struct {
		Spoken   string `json:"spoken"`
		BundleID string `json:"bundle_id"`
	}

	appsMu.Lock()
	var rows []entry
	for _, app := range apps {
		for _, alias := range app.Aliases {
			spoken := strings.ToLower(alias)
			rows = append(rows, entry{Spoken: spoken, BundleID: app.BundleID})
		}
	}
	appsMu.Unlock()

	records := make([]shared.CollectionPutEntry, 0, len(rows))
	for _, row := range rows {
		raw, err := json.Marshal(row)
		if err != nil {
			shared.Logf("system", "apps: marshal %q: %v", row.Spoken, err)
			return
		}
		records = append(records, shared.CollectionPutEntry{ID: row.Spoken, Payload: raw})
	}

	if _, err := p.Replace("apps", records, shared.ScopeCollection()); err != nil {
		shared.Logf("system", "replace apps: %v", err)
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

	// Suppress/restore each alias via platform override
	for _, alias := range aliases {
		spoken := strings.ToLower(alias)
		if nowEnabled {
			// Re-enable: clear the suppression so plugin data takes effect.
			// tenant "_user": this is the user's settings-tab gesture, the
			// plugin is transport — it must land in the user band, not ours.
			if err := p.Call("overrides.apply", map[string]any{
				"collection": "apps", "action": "restore", "id": spoken, "tenant": "_user",
			}, nil); err != nil {
				shared.Logf("system", "app_toggle restore %q: %v", spoken, err)
			}
		} else {
			// Disable: suppress each alias
			if err := p.Call("overrides.apply", map[string]any{
				"collection": "apps", "action": "remove", "id": spoken, "tenant": "_user",
			}, nil); err != nil {
				shared.Logf("system", "app_toggle remove %q: %v", spoken, err)
			}
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

	// Add via platform override — a user gesture, so the user band.
	if err := p.Call("overrides.apply", map[string]any{
		"collection": "apps", "action": "add", "tenant": "_user",
		"fields": map[string]string{"spoken": alias, "bundle_id": bundleID},
	}, nil); err != nil {
		shared.Logf("system", "alias add %q: %v", alias, err)
	}
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

	// Remove via platform override — "remove" filters the merged view, so it
	// suppresses a plugin-shipped alias and drops a user-added one alike.
	// A user gesture, so the user band.
	if err := p.Call("overrides.apply", map[string]any{
		"collection": "apps", "action": "remove", "id": alias, "tenant": "_user",
	}, nil); err != nil {
		shared.Logf("system", "alias remove %q: %v", alias, err)
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
