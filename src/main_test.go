package main

import (
	"strings"
	"testing"

	"github.com/branchkit/plugin-sdk-go"
)

// --- jsEscape ---

func TestJsEscape_Backslash(t *testing.T) {
	got := jsEscape(`a\b`)
	if got != `a\\b` {
		t.Errorf("expected a\\\\b, got %q", got)
	}
}

func TestJsEscape_SingleQuote(t *testing.T) {
	got := jsEscape("it's")
	if got != `it\'s` {
		t.Errorf("expected it\\'s, got %q", got)
	}
}

func TestJsEscape_Both(t *testing.T) {
	got := jsEscape(`it's a\path`)
	want := `it\'s a\\path`
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestJsEscape_Empty(t *testing.T) {
	if got := jsEscape(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

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
func setTestApps(entries []AppEntry) {
	appsMu.Lock()
	apps = entries
	appsMu.Unlock()
}

// --- handleRenderSettings (apps tab) ---

func TestHandleRenderSettings_AppsTab(t *testing.T) {
	setTestApps([]AppEntry{
		{Name: "Safari", BundleID: "com.apple.Safari", Enabled: true, Aliases: []string{"browser"}},
	})
	req := &branchkit.RenderSettingsRequest{TabKey: "apps"}
	result, err := handleRenderSettings(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(branchkit.RenderSettingsResponse)
	if resp.HTML == "" {
		t.Error("expected non-empty HTML for apps tab")
	}
	if !strings.Contains(resp.HTML, "Safari") {
		t.Error("expected app name in rendered HTML")
	}
	if !strings.Contains(resp.HTML, "browser") {
		t.Error("expected alias in rendered HTML")
	}
}

func TestHandleRenderSettings_AppsSearch(t *testing.T) {
	setTestApps([]AppEntry{
		{Name: "Safari", BundleID: "com.apple.Safari", Enabled: true},
		{Name: "Finder", BundleID: "com.apple.finder", Enabled: true},
	})
	req := &branchkit.RenderSettingsRequest{
		TabKey: "apps",
		Search: "safari",
	}
	result, err := handleRenderSettings(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(branchkit.RenderSettingsResponse)
	if !strings.Contains(resp.HTML, "Safari") {
		t.Error("expected Safari in filtered results")
	}
	if strings.Contains(resp.HTML, "Finder") {
		t.Error("expected Finder to be filtered out by search")
	}
}

func TestHandleRenderSettings_UnknownTab(t *testing.T) {
	req := &branchkit.RenderSettingsRequest{TabKey: "nonexistent"}
	result, err := handleRenderSettings(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(branchkit.RenderSettingsResponse)
	if resp.HTML != "" {
		t.Errorf("expected empty HTML for unknown tab, got %q", resp.HTML)
	}
}

// --- per-action no-op input handling ---
//
// Routing of unknown actions and "did the dispatch happen" semantics live in
// the SDK's HandleAction tests (plugin-sdk-go/actions_test.go). These tests
// only cover plugin-local input validation paths.

func TestHandleSetOutput_NoName(t *testing.T) {
	req := &branchkit.OnActionRequest{Action: "system.set_output"}
	if _, err := handleSetOutput(SetOutputParams{}, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleSetInput_NoName(t *testing.T) {
	req := &branchkit.OnActionRequest{Action: "system.set_input"}
	if _, err := handleSetInput(SetInputParams{}, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleLaunch_NoBundleID(t *testing.T) {
	req := &branchkit.OnActionRequest{Action: "system.launch"}
	if _, err := handleLaunch(LaunchParams{}, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleOpen_NoTarget(t *testing.T) {
	req := &branchkit.OnActionRequest{Action: "system.open"}
	if _, err := handleOpen(OpenParams{}, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- templ rendering ---

func TestAppsTempl_RendersNonEmpty(t *testing.T) {
	rows := []appRowView{
		{Name: "Safari", BundleID: "com.apple.Safari", Aliases: []string{"browser"}, Status: "Enabled", BadgeClass: "badge-core"},
	}
	html := renderTempl(Apps(rows, false))
	if html == "" {
		t.Fatal("expected non-empty HTML from Apps templ")
	}
	if !strings.Contains(html, "Safari") {
		t.Error("expected app name in rendered output")
	}
	if !strings.Contains(html, "browser") {
		t.Error("expected alias in rendered output")
	}
}

func TestAppsTempl_EmptyList(t *testing.T) {
	html := renderTempl(Apps(nil, true))
	if html == "" {
		t.Fatal("expected non-empty HTML even with empty list")
	}
	if !strings.Contains(html, "settings-table") {
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
	html := renderTempl(Sound(data))
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
	html := renderTempl(Sound(data))
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
	html := renderTempl(Sound(data))
	if html == "" {
		t.Fatal("expected non-empty HTML even with no devices")
	}
	// Should NOT contain device sections when lists are empty
	if strings.Contains(html, "Output Devices") {
		t.Error("expected no output devices section when list is empty")
	}
}

// --- appRowView construction in handleRenderSettings ---

func TestAppRowView_DisabledStatus(t *testing.T) {
	setTestApps([]AppEntry{
		{Name: "Hidden", BundleID: "com.example.hidden", Enabled: false},
	})
	req := &branchkit.RenderSettingsRequest{TabKey: "apps"}
	result, err := handleRenderSettings(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(branchkit.RenderSettingsResponse)
	if !strings.Contains(resp.HTML, "Disabled") {
		t.Error("expected 'Disabled' status badge for disabled app")
	}
}

func TestHandleRenderSettings_SearchByBundleID(t *testing.T) {
	setTestApps([]AppEntry{
		{Name: "Safari", BundleID: "com.apple.Safari", Enabled: true},
		{Name: "Finder", BundleID: "com.apple.finder", Enabled: true},
	})
	req := &branchkit.RenderSettingsRequest{
		TabKey: "apps",
		Search: "com.apple.Safari",
	}
	result, err := handleRenderSettings(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(branchkit.RenderSettingsResponse)
	if !strings.Contains(resp.HTML, "Safari") {
		t.Error("expected Safari matched by bundle ID search")
	}
	if strings.Contains(resp.HTML, "Finder") {
		t.Error("expected Finder filtered out")
	}
}

// --- Action launch param compatibility ---

func TestHandleLaunch_EmptyBundleIDNoOp(t *testing.T) {
	// The canonical key is "bundle_id" (matches the manifest and the
	// generated LaunchParams struct). With an empty value, the handler
	// logs and returns without making an RPC call.
	req := &branchkit.OnActionRequest{Action: "system.launch"}
	if _, err := handleLaunch(LaunchParams{BundleID: ""}, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
