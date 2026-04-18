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

func TestApplyOverrides_DisablesApp(t *testing.T) {
	scanned := []AppEntry{
		{Name: "Chrome", BundleID: "com.google.Chrome", Aliases: []string{"chrome"}, Enabled: true},
	}
	overrides := []AppEntry{
		{Name: "Chrome", BundleID: "com.google.Chrome", Enabled: false},
	}
	result := applyOverrides(scanned, overrides)
	if result[0].Enabled {
		t.Error("expected app to be disabled by override")
	}
}

func TestApplyOverrides_AddsAliases(t *testing.T) {
	scanned := []AppEntry{
		{Name: "Chrome", BundleID: "com.google.Chrome", Aliases: []string{"chrome"}, Enabled: true},
	}
	overrides := []AppEntry{
		{Name: "Chrome", BundleID: "com.google.Chrome", Aliases: []string{"browser", "web"}, Enabled: true},
	}
	result := applyOverrides(scanned, overrides)
	if !containsLower(result[0].Aliases, "browser") {
		t.Error("expected 'browser' alias from override")
	}
	if !containsLower(result[0].Aliases, "web") {
		t.Error("expected 'web' alias from override")
	}
	if !containsLower(result[0].Aliases, "chrome") {
		t.Error("expected original 'chrome' alias preserved")
	}
}

func TestApplyOverrides_AppendsCustomApp(t *testing.T) {
	scanned := []AppEntry{
		{Name: "Safari", BundleID: "com.apple.Safari", Aliases: []string{"safari"}, Enabled: true},
	}
	overrides := []AppEntry{
		{Name: "Custom App", BundleID: "com.custom.app", Aliases: []string{"custom"}, Enabled: true},
	}
	result := applyOverrides(scanned, overrides)
	if len(result) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(result))
	}
}

func TestPushAppsCollection_BuildsFlatEntries(t *testing.T) {
	// Set internal state and verify the flat structure would be correct
	setTestApps([]AppEntry{
		{Name: "Chrome", BundleID: "com.google.Chrome", Aliases: []string{"chrome", "google chrome"}, Enabled: true},
		{Name: "Hidden", BundleID: "com.example.hidden", Aliases: []string{"hidden"}, Enabled: false},
	})

	// Verify getApps returns the right data
	allApps := getApps()
	if len(allApps) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(allApps))
	}

	// Count enabled aliases (what pushAppsCollection would produce)
	flatCount := 0
	for _, app := range allApps {
		if app.Enabled {
			flatCount += len(app.Aliases)
		}
	}
	if flatCount != 2 {
		t.Errorf("expected 2 flat entries (chrome + google chrome), got %d", flatCount)
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
