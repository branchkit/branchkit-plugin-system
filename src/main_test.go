package main

import (
	"strings"
	"testing"

	"github.com/branchkit/plugin-sdk-go"
)

// --- voiceHint ---

func TestVoiceHint_MacBookAir(t *testing.T) {
	got := voiceHint("MacBook Air Speakers")
	if got != "speakers" {
		t.Errorf("expected 'speakers', got %q", got)
	}
}

func TestVoiceHint_MacBookPro(t *testing.T) {
	got := voiceHint("MacBook Pro Microphone")
	if got != "microphone" {
		t.Errorf("expected 'microphone', got %q", got)
	}
}

func TestVoiceHint_BuiltIn(t *testing.T) {
	got := voiceHint("Built-in Output")
	if got != "output" {
		t.Errorf("expected 'output', got %q", got)
	}
}

func TestVoiceHint_ExternalDevice(t *testing.T) {
	got := voiceHint("Sony WH-1000XM5")
	if got != "sony wh-1000xm5" {
		t.Errorf("expected lowercase passthrough, got %q", got)
	}
}

// --- matchesDevice ---

func TestMatchesDevice_ByName(t *testing.T) {
	d := branchkit.AudioDevice{Name: "MacBook Air Speakers", UID: "spk-1"}
	if !matchesDevice(d, "speakers", nil) {
		t.Error("expected match by name substring")
	}
}

func TestMatchesDevice_ByAlias(t *testing.T) {
	d := branchkit.AudioDevice{Name: "Sony WH-1000XM5", UID: "sony-1"}
	aliases := map[string][]string{"sony-1": {"headphones"}}
	if !matchesDevice(d, "headphones", aliases) {
		t.Error("expected match by alias")
	}
}

func TestMatchesDevice_NoMatch(t *testing.T) {
	d := branchkit.AudioDevice{Name: "AirPods Pro", UID: "airpods-1"}
	if matchesDevice(d, "speakers", nil) {
		t.Error("expected no match")
	}
}

func TestMatchesDevice_NilAliases(t *testing.T) {
	d := branchkit.AudioDevice{Name: "AirPods Pro", UID: "airpods-1"}
	if matchesDevice(d, "airpods", nil) != true {
		t.Error("expected match by name with nil aliases")
	}
}

// setTestApps sets the internal app list for testing.
func (h *Host) setTestApps(entries []AppEntry) {
	h.appsMu.Lock()
	h.apps = entries
	h.appsMu.Unlock()
}

// --- renderAppsTab ---

func TestHandleRenderSettings_AppsTab(t *testing.T) {
	h := newTestHost()
	h.setTestApps([]AppEntry{
		{Name: "Safari", BundleID: "com.apple.Safari", Enabled: true, Aliases: []string{"browser"}},
	})
	req := &branchkit.RenderSettingsRequest{TabKey: "apps"}
	html, err := h.renderAppsTab(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if html == "" {
		t.Error("expected non-empty HTML for apps tab")
	}
	if !strings.Contains(html, "Safari") {
		t.Error("expected app name in rendered HTML")
	}
	if !strings.Contains(html, "browser") {
		t.Error("expected alias in rendered HTML")
	}
}

func TestHandleRenderSettings_AppsSearch(t *testing.T) {
	h := newTestHost()
	h.setTestApps([]AppEntry{
		{Name: "Safari", BundleID: "com.apple.Safari", Enabled: true},
		{Name: "Finder", BundleID: "com.apple.finder", Enabled: true},
	})
	req := &branchkit.RenderSettingsRequest{
		TabKey: "apps",
		Search: "safari",
	}
	html, err := h.renderAppsTab(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "Safari") {
		t.Error("expected Safari in filtered results")
	}
	if strings.Contains(html, "Finder") {
		t.Error("expected Finder to be filtered out by search")
	}
}

// --- per-action no-op input handling ---
//
// Routing of unknown actions and "did the dispatch happen" semantics live in
// the SDK's HandleAction tests (plugin-sdk-go/actions_test.go). These tests
// only cover plugin-local input validation paths.

func TestHandleSetOutput_NoName(t *testing.T) {
	h := newTestHost()
	req := &branchkit.OnActionRequest{Action: "system.set_output"}
	if _, err := h.handleSetOutput(SetOutputParams{}, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleSetInput_NoName(t *testing.T) {
	h := newTestHost()
	req := &branchkit.OnActionRequest{Action: "system.set_input"}
	if _, err := h.handleSetInput(SetInputParams{}, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleLaunch_NoBundleID(t *testing.T) {
	h := newTestHost()
	req := &branchkit.OnActionRequest{Action: "system.launch"}
	if _, err := h.handleLaunch(LaunchParams{}, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleOpen_NoTarget(t *testing.T) {
	h := newTestHost()
	req := &branchkit.OnActionRequest{Action: "system.open"}
	if _, err := h.handleOpen(OpenParams{}, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- templ rendering ---

func TestAppsTempl_RendersNonEmpty(t *testing.T) {
	rows := []appRowView{
		{Name: "Safari", BundleID: "com.apple.Safari", Aliases: []string{"browser"},
			Traits: []string{"terminal"}, Status: "Enabled", BadgeClass: "badge-core"},
	}
	cat := traitCatalog{
		ByBundle: map[string][]string{"com.apple.Safari": {"terminal"}},
		Counts:   map[string]int{"terminal": 7, "chat": 7},
		Names:    []string{"chat", "terminal"},
		Loaded:   true,
	}
	html := mustRender(t, Apps(rows, false, cat))
	if html == "" {
		t.Fatal("expected non-empty HTML from Apps templ")
	}
	if !strings.Contains(html, "Safari") {
		t.Error("expected app name in rendered output")
	}
	if !strings.Contains(html, "browser") {
		t.Error("expected alias in rendered output")
	}
	if !strings.Contains(html, "app_trait_remove") {
		t.Error("expected a trait chip with its remove button")
	}
	// The add menu carries counts — the only thing that makes a misspelled
	// trait visible next to the one it was meant to be.
	if !strings.Contains(html, "terminal") || !strings.Contains(html, "(7)") {
		t.Error("expected trait options labelled with their app counts")
	}
	if !strings.Contains(html, "__new__") {
		t.Error("expected the 'New trait…' escape hatch in the menu")
	}
	// The chip tooltip names the trait, which means templ must INTERPOLATE it.
	// A `title="… { trait } …"` plain-string attribute renders the braces
	// literally, and nothing else in the suite would have noticed — the page
	// looks right until you hover it. Found exactly that way.
	if strings.Contains(html, "{ trait }") {
		t.Error("tooltip rendered the templ expression literally — use title={ \"…\" + trait } for attribute interpolation")
	}
	// templ escapes the apostrophes, so match the escaped form — the point is
	// that "terminal" appears INSIDE the tooltip, not that braces are absent.
	if !strings.Contains(html, "This app is a &#39;terminal&#39;") {
		t.Error("expected the trait name interpolated into the chip tooltip")
	}
	// The tooltip is where the fact-vs-opinion distinction is stated, and it
	// is an obligation rather than a nicety: a trait is a fact every plugin
	// reads, so removing one is never a dictation-only change.
	if !strings.Contains(html, "not just dictation") {
		t.Error("tooltip must say a trait change reaches every plugin, not only dictation")
	}
}

func TestAppsTempl_EmptyList(t *testing.T) {
	html := mustRender(t, Apps(nil, true, traitCatalog{}))
	if html == "" {
		t.Fatal("expected non-empty HTML even with empty list")
	}
	if !strings.Contains(html, "<bk-table") {
		t.Error("expected table container in output")
	}
}

func TestSoundTempl_RendersNonEmpty(t *testing.T) {
	data := soundSettingsData{
		Volume:      50,
		VolumeMinus: 43,
		VolumePlus:  57,
		Muted:       false,
		Outputs: []deviceView{
			{UID: "spk-1", Name: "MacBook Air Speakers", VoiceHint: "speakers", IsDefault: true},
		},
		Inputs: []deviceView{
			{UID: "mic-1", Name: "MacBook Air Microphone", VoiceHint: "microphone", IsDefault: true},
		},
	}
	html := mustRender(t, Sound(data))
	if html == "" {
		t.Fatal("expected non-empty HTML from Sound templ")
	}
	if !strings.Contains(html, "50%") {
		t.Error("expected volume percentage in output")
	}
	if !strings.Contains(html, "MacBook Air Speakers") {
		t.Error("expected output device name")
	}
	if !strings.Contains(html, "MacBook Air Microphone") {
		t.Error("expected input device name")
	}
}

func TestSoundTempl_Muted(t *testing.T) {
	data := soundSettingsData{
		Volume:      50,
		VolumeMinus: 43,
		VolumePlus:  57,
		Muted:       true,
	}
	html := mustRender(t, Sound(data))
	if html == "" {
		t.Fatal("expected non-empty HTML")
	}
	// When muted, the "On" button should be bold (active state)
	if !strings.Contains(html, "Mute") || !strings.Contains(html, "On") {
		t.Error("expected mute controls in output")
	}
}

func TestSoundTempl_NoDevices(t *testing.T) {
	data := soundSettingsData{
		Volume:      0,
		VolumeMinus: 0,
		VolumePlus:  7,
	}
	html := mustRender(t, Sound(data))
	if html == "" {
		t.Fatal("expected non-empty HTML even with no devices")
	}
	// Should NOT contain device sections when lists are empty
	if strings.Contains(html, "Output Devices") {
		t.Error("expected no output devices section when list is empty")
	}
}

// --- appRowView construction in renderAppsTab ---

func TestAppRowView_DisabledStatus(t *testing.T) {
	h := newTestHost()
	h.setTestApps([]AppEntry{
		{Name: "Hidden", BundleID: "com.example.hidden", Enabled: false},
	})
	req := &branchkit.RenderSettingsRequest{TabKey: "apps"}
	html, err := h.renderAppsTab(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "Disabled") {
		t.Error("expected 'Disabled' status badge for disabled app")
	}
}

func TestHandleRenderSettings_SearchByBundleID(t *testing.T) {
	h := newTestHost()
	h.setTestApps([]AppEntry{
		{Name: "Safari", BundleID: "com.apple.Safari", Enabled: true},
		{Name: "Finder", BundleID: "com.apple.finder", Enabled: true},
	})
	req := &branchkit.RenderSettingsRequest{
		TabKey: "apps",
		Search: "com.apple.Safari",
	}
	html, err := h.renderAppsTab(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "Safari") {
		t.Error("expected Safari matched by bundle ID search")
	}
	if strings.Contains(html, "Finder") {
		t.Error("expected Finder filtered out")
	}
}

// --- Action launch param compatibility ---

func TestHandleLaunch_EmptyBundleIDNoOp(t *testing.T) {
	h := newTestHost()
	// The canonical key is "bundle_id" (matches the manifest and the
	// generated LaunchParams struct). With an empty value, the handler
	// logs and returns without making an RPC call.
	req := &branchkit.OnActionRequest{Action: "system.launch"}
	if _, err := h.handleLaunch(LaunchParams{BundleID: ""}, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// mustRender is the test-side render: a component that fails to render is
// a test failure, not an empty string.
func mustRender(t *testing.T, c branchkit.HTMLComponent) string {
	t.Helper()
	html, err := branchkit.RenderComponent(c)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return html
}
