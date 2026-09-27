package main

import (
	"testing"
)

func TestMergeAliases_AddsToCoreApps(t *testing.T) {
	scanned := []AppEntry{
		{Name: "Code", BundleID: "com.microsoft.VSCode", Aliases: []string{"code"}, Enabled: true},
	}
	core := []AppEntry{
		{Name: "Visual Studio Code", BundleID: "com.microsoft.VSCode", Aliases: []string{"code", "vs code", "vscode"}, Enabled: true},
	}
	result := mergeAliases(scanned, core)
	if len(result) != 1 {
		t.Fatalf("expected 1 app, got %d", len(result))
	}
	aliases := result[0].Aliases
	if !containsLower(aliases, "vs code") {
		t.Error("expected 'vs code' alias to be merged")
	}
	if !containsLower(aliases, "vscode") {
		t.Error("expected 'vscode' alias to be merged")
	}
}

func TestMergeAliases_AppendsUninstalledCoreApps(t *testing.T) {
	scanned := []AppEntry{
		{Name: "Safari", BundleID: "com.apple.Safari", Aliases: []string{"safari"}, Enabled: true},
	}
	core := []AppEntry{
		{Name: "Spotify", BundleID: "com.spotify.client", Aliases: []string{"spotify"}, Enabled: true},
	}
	result := mergeAliases(scanned, core)
	if len(result) != 2 {
		t.Fatalf("expected 2 apps (scanned + uninstalled core), got %d", len(result))
	}
	if result[1].BundleID != "com.spotify.client" {
		t.Errorf("expected appended core app, got %q", result[1].BundleID)
	}
}

func TestMergeAliases_NoDuplicateAliases(t *testing.T) {
	scanned := []AppEntry{
		{Name: "Chrome", BundleID: "com.google.Chrome", Aliases: []string{"chrome", "google chrome"}, Enabled: true},
	}
	core := []AppEntry{
		{Name: "Google Chrome", BundleID: "com.google.Chrome", Aliases: []string{"chrome", "google chrome"}, Enabled: true},
	}
	result := mergeAliases(scanned, core)
	count := 0
	for _, a := range result[0].Aliases {
		if a == "chrome" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'chrome' alias once, got %d times", count)
	}
}

func TestPushAppsCollection_BuildsFlatEntries(t *testing.T) {
	h := newTestHost()
	h.setTestApps([]AppEntry{
		{Name: "Chrome", BundleID: "com.google.Chrome", Aliases: []string{"chrome", "google chrome"}, Enabled: true},
		{Name: "Hidden", BundleID: "com.example.hidden", Aliases: []string{"hidden"}, Enabled: false},
	})

	allApps := h.getApps()
	if len(allApps) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(allApps))
	}

	// pushAppsCollection now pushes ALL apps (platform overrides handle disabling)
	flatCount := 0
	for _, app := range allApps {
		flatCount += len(app.Aliases)
	}
	if flatCount != 3 {
		t.Errorf("expected 3 flat entries (chrome + google chrome + hidden), got %d", flatCount)
	}
}

func TestContainsLower(t *testing.T) {
	ss := []string{"Chrome", "Google Chrome"}
	if !containsLower(ss, "chrome") {
		t.Error("expected case-insensitive match")
	}
	if !containsLower(ss, "google chrome") {
		t.Error("expected multi-word match")
	}
	if containsLower(ss, "firefox") {
		t.Error("expected no match for firefox")
	}
	if containsLower(nil, "anything") {
		t.Error("expected no match on nil slice")
	}
}

// Off macOS a curated entry applies only through its id for that OS, and
// then carries that id, so its aliases land on the app the scan found. Keyed
// by the macOS bundle id it became a second "firefox" row that won the
// spoken name and launched nothing on Windows.
func TestCoreForOS(t *testing.T) {
	core := []AppEntry{
		{Name: "Firefox", BundleID: "org.mozilla.firefox", Aliases: []string{"firefox"},
			IDs: map[string][]string{"linux": {"firefox"}, "windows": {"firefox"}}},
		{Name: "Safari", BundleID: "com.apple.Safari", Aliases: []string{"safari"}},
		{Name: "kitty", BundleID: "net.kovidgoyal.kitty", Traits: []string{"terminal"},
			IDs: map[string][]string{"linux": {"kitty"}}},
	}
	if got := coreForOS(core, "darwin"); len(got) != 3 || got[0].BundleID != "org.mozilla.firefox" {
		t.Fatalf("macOS keeps the file as written, got %+v", got)
	}
	win := coreForOS(core, "windows")
	if len(win) != 1 || win[0].BundleID != "firefox" || win[0].IDs != nil {
		t.Fatalf("windows: want only firefox, got %+v", win)
	}
	linux := coreForOS(core, "linux")
	if len(linux) != 2 || linux[1].BundleID != "kitty" || linux[1].Traits[0] != "terminal" {
		t.Fatalf("linux: want firefox and kitty, got %+v", linux)
	}

	scanned := []AppEntry{{Name: "Firefox", BundleID: "firefox", Aliases: []string{"firefox"}}}
	merged := mergeAliases(scanned, win)
	if len(merged) != 1 || merged[0].BundleID != "firefox" {
		t.Fatalf("the curated entry must merge into the scanned app, got %+v", merged)
	}
}
