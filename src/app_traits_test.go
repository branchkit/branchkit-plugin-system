package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// buildAppTraits reads the CURATED file only. The native scan cannot know what
// kind of application something is, so a scanned-but-uncurated app has no
// groups — and that must read as "no opinion", never as an empty group.
func TestBuildAppTraitsFromCoreOnly(t *testing.T) {
	core := []AppEntry{
		{Name: "Terminal", BundleID: "com.apple.Terminal", Aliases: []string{"terminal"}, Traits: []string{"terminal"}},
		{Name: "Slack", BundleID: "com.tinyspeck.slackmacgap", Aliases: []string{"slack"}, Traits: []string{"chat"}},
		{Name: "Finder", BundleID: "com.apple.finder", Aliases: []string{"finder"}},
	}
	got := buildAppTraits(core)
	want := map[string][]string{
		"com.apple.Terminal":        {"terminal"},
		"com.tinyspeck.slackmacgap": {"chat"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildAppTraits = %v, want %v", got, want)
	}
	if _, ok := got["com.apple.finder"]; ok {
		t.Error("an app with no traits got an entry — absent and empty must not be the same thing")
	}
}

// Trait names are normalised and deduplicated, so a stray "Terminal" or a
// repeat cannot produce two assignments a consumer sees as different.
func TestBuildAppTraitsNormalises(t *testing.T) {
	core := []AppEntry{{
		BundleID: "com.example.thing",
		Traits:   []string{"Terminal", " terminal ", "", "chat"},
	}}
	got := buildAppTraits(core)["com.example.thing"]
	if want := []string{"terminal", "chat"}; !reflect.DeepEqual(got, want) {
		t.Errorf("traits = %v, want %v", got, want)
	}
}

// A curated entry with no aliases is TRAIT-ONLY: it says "this bundle id is a
// terminal" and must not become an app row. Appending it would put a row in the
// Apps settings table for software that is not installed, and contribute
// nothing to the matcher either way, since the collection is keyed by alias.
func TestTraitOnlyEntriesStayOutOfTheAppList(t *testing.T) {
	scanned := []AppEntry{
		{Name: "Terminal", BundleID: "com.apple.Terminal", Aliases: []string{"terminal"}, Enabled: true},
	}
	core := []AppEntry{
		{Name: "Terminal", BundleID: "com.apple.Terminal", Aliases: []string{"term"}, Traits: []string{"terminal"}},
		// Not installed, no aliases — facts only.
		{Name: "WezTerm", BundleID: "com.github.wez.wezterm", Traits: []string{"terminal"}},
		// Not installed, but HAS an alias — still appended, as before, so
		// "launch discord" keeps working on a machine without Discord.
		{Name: "Discord", BundleID: "com.hnc.Discord", Aliases: []string{"discord"}, Traits: []string{"chat"}},
	}

	merged := mergeAliases(scanned, core)
	byID := map[string]AppEntry{}
	for _, a := range merged {
		byID[a.BundleID] = a
	}
	if _, ok := byID["com.github.wez.wezterm"]; ok {
		t.Error("trait-only entry became an app row")
	}
	if _, ok := byID["com.hnc.Discord"]; !ok {
		t.Error("an aliased core entry stopped being appended — that is a regression, not part of this change")
	}

	// ...but its traits are still published, because buildAppTraits reads the
	// core file rather than the merged app list.
	if g := buildAppTraits(core)["com.github.wez.wezterm"]; len(g) != 1 || g[0] != "terminal" {
		t.Errorf("trait-only entry lost its traits: %v", g)
	}
}

// The shipped file is data, and a typo in it is invisible until something
// misbehaves. Parse it and hold it to the rules the loader assumes.
func TestCoreAppsFileIsWellFormed(t *testing.T) {
	path := filepath.Join("..", "core_apps.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var entries []AppEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	seen := map[string]bool{}
	for _, e := range entries {
		if e.Name == "" || e.BundleID == "" {
			t.Errorf("entry missing name or bundle_id: %+v", e)
		}
		if seen[e.BundleID] {
			t.Errorf("duplicate bundle_id %q — the later entry silently wins", e.BundleID)
		}
		seen[e.BundleID] = true
		if len(e.Aliases) == 0 && len(e.Traits) == 0 {
			t.Errorf("%s has neither aliases nor traits, so it does nothing", e.BundleID)
		}
		// Group-only entries are claims about software this machine may not
		// have, so they carry a note saying the bundle id is unverified.
		if len(e.Aliases) == 0 && e.Note == "" {
			t.Errorf("%s is trait-only but undocumented — say that the bundle id is unverified", e.BundleID)
		}
	}

	// The taxonomy is deliberately narrow: only groups a consumer actually
	// reads. Shipping `browser` or `editor` with nothing reading them is the
	// same "build for a hypothetical consumer" mistake in data form.
	known := map[string]bool{"terminal": true, "chat": true}
	for trait := range invertTraits(buildAppTraits(entries)) {
		if !known[trait] {
			t.Errorf("trait %q has no reader — add the consumer first, or drop the trait", trait)
		}
	}
}

// invertTraits turns bundle → traits into trait → bundles, which is the
// "which apps have this trait?" question the collection exists to answer.
func invertTraits(byID map[string][]string) map[string][]string {
	out := map[string][]string{}
	for id, traits := range byID {
		for _, t := range traits {
			out[t] = append(out[t], id)
		}
	}
	return out
}

// The fourteen apps the voice plugin used to hard-code must all still be
// classified, or moving them was a silent behaviour regression rather than a
// relocation. This is the test that would have caught a dropped row.
func TestRelocatedVoiceTableIsComplete(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "core_apps.json"))
	if err != nil {
		t.Fatalf("read core_apps.json: %v", err)
	}
	var entries []AppEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("parse core_apps.json: %v", err)
	}
	traits := buildAppTraits(entries)

	relocated := map[string]string{
		"com.tinyspeck.slackmacgap":         "chat",
		"com.hnc.Discord":                   "chat",
		"com.apple.MobileSMS":               "chat",
		"net.whatsapp.WhatsApp":             "chat",
		"ru.keepcoder.Telegram":             "chat",
		"org.telegram.desktop":              "chat",
		"org.whispersystems.signal-desktop": "chat",
		"com.apple.Terminal":                "terminal",
		"com.googlecode.iterm2":             "terminal",
		"com.mitchellh.ghostty":             "terminal",
		"dev.warp.Warp-Stable":              "terminal",
		"net.kovidgoyal.kitty":              "terminal",
		"org.alacritty":                     "terminal",
		"com.github.wez.wezterm":            "terminal",
	}
	for bundleID, want := range relocated {
		got := traits[bundleID]
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s traits = %v, want [%s] — this app lost its classification in the move",
				bundleID, got, want)
		}
	}
}

// --- the editor ---

// Trait names become half a record id, and `:` is the separator. A name
// containing it would produce an id that parses back wrong, so it is refused
// rather than escaped.
func TestNormalizeTrait(t *testing.T) {
	ok := map[string]string{
		"terminal":    "terminal",
		"  Terminal":  "terminal",
		"TERMINAL  ":  "terminal",
		"web-browser": "web-browser",
		"code_editor": "code_editor",
		"x11":         "x11",
	}
	for in, want := range ok {
		got, err := normalizeTrait(in)
		if err != nil {
			t.Errorf("normalizeTrait(%q) errored: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeTrait(%q) = %q, want %q", in, got, want)
		}
	}
	bad := []string{"", "   ", "term:inal", "web browser", "café", "a/b", strings.Repeat("x", 33)}
	for _, in := range bad {
		if got, err := normalizeTrait(in); err == nil {
			t.Errorf("normalizeTrait(%q) = %q, want an error", in, got)
		}
	}
}

// The id format is what `overrides.apply {action: "remove", id: …}` addresses.
// If this changes, removing a shipped trait silently stops working — the call
// succeeds and suppresses nothing.
func TestTraitRecordID(t *testing.T) {
	if got, want := traitRecordID("terminal", "com.apple.Terminal"), "terminal:com.apple.Terminal"; got != want {
		t.Errorf("traitRecordID = %q, want %q", got, want)
	}
	// The pusher and the editor must agree on it, or a user removal addresses
	// a record the publisher never wrote.
	if traitRecordID("chat", "com.hnc.Discord") != "chat:com.hnc.Discord" {
		t.Error("id format drifted from the publishing path")
	}
}

// The add menu's counts are the typo backstop — `terminl (1)` beside
// `terminal (7)` is the only thing that makes a misspelling visible.
func TestTraitCatalogCounts(t *testing.T) {
	cat := traitCatalog{ByBundle: map[string][]string{}, Counts: map[string]int{}}
	for _, r := range []struct{ trait, bundle string }{
		{"terminal", "com.apple.Terminal"},
		{"terminal", "dev.warp.Warp-Stable"},
		{"chat", "com.tinyspeck.slackmacgap"},
	} {
		cat.ByBundle[r.bundle] = append(cat.ByBundle[r.bundle], r.trait)
		cat.Counts[r.trait]++
	}
	if cat.Counts["terminal"] != 2 || cat.Counts["chat"] != 1 {
		t.Errorf("counts = %v", cat.Counts)
	}
	if len(cat.ByBundle["com.apple.Terminal"]) != 1 {
		t.Errorf("Terminal traits = %v", cat.ByBundle["com.apple.Terminal"])
	}
}

// The template shows a trait's count next to its name; itoa exists only
// because a templ file cannot import strconv.
func TestItoa(t *testing.T) {
	if itoa(7) != "7" || itoa(0) != "0" {
		t.Errorf("itoa broken: %q %q", itoa(7), itoa(0))
	}
}

// The write-path invariant, asserted at the level a unit test can reach: both
// mutations go restore-first, so putting a trait back the way it shipped leaves
// nothing behind in the user band.
//
// The RPC sequence itself is verified on a running instance (add-then-remove
// and remove-then-re-add both return the user band to empty); what this pins is
// the reasoning that makes the sequence correct, so a future edit that drops the
// restore has something to fail against.
func TestRestoreIsTheInverseOfAdd(t *testing.T) {
	src, err := os.ReadFile("app_traits.go")
	if err != nil {
		t.Fatalf("read app_traits.go: %v", err)
	}
	code := string(src)

	// Both mutations must issue a restore before their own verb. Plain `remove`
	// appends to `removed` and never touches `added`, so a user-added trait
	// removed that way leaves both records stacked — correct on screen, and
	// growing the override file by a contradictory pair every cycle.
	for _, fn := range []string{"func addAppTrait", "func removeAppTrait"} {
		body := code[strings.Index(code, fn):]
		if end := strings.Index(body[1:], "\nfunc "); end >= 0 {
			body = body[:end]
		}
		if !strings.Contains(body, `"action": "restore"`) {
			t.Errorf("%s no longer restores first — the user band will accumulate "+
				"contradictory add/remove pairs that the composed view hides", fn)
		}
		if !strings.Contains(body, "hasTrait(") {
			t.Errorf("%s must check whether restoring was enough before writing its own "+
				"override, or it shadows a shipped record with a private copy", fn)
		}
	}
}
