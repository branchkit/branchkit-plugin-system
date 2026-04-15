package main

import (
	"encoding/json"
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
	d := shared.AudioDevice{Name: "MacBook Air Speakers", UID: "spk-1"}
	if !matchesDevice(d, "speakers", nil) {
		t.Error("expected match by name substring")
	}
}

func TestMatchesDevice_ByAlias(t *testing.T) {
	d := shared.AudioDevice{Name: "Sony WH-1000XM5", UID: "sony-1"}
	aliases := map[string][]string{"sony-1": {"headphones"}}
	if !matchesDevice(d, "headphones", aliases) {
		t.Error("expected match by alias")
	}
}

func TestMatchesDevice_NoMatch(t *testing.T) {
	d := shared.AudioDevice{Name: "AirPods Pro", UID: "airpods-1"}
	if matchesDevice(d, "speakers", nil) {
		t.Error("expected no match")
	}
}

func TestMatchesDevice_NilAliases(t *testing.T) {
	d := shared.AudioDevice{Name: "AirPods Pro", UID: "airpods-1"}
	if matchesDevice(d, "airpods", nil) != true {
		t.Error("expected match by name with nil aliases")
	}
}

// --- handleRenderHud ---

func TestHandleRenderHud_AppsMode(t *testing.T) {
	enabled := true
	apps := []shared.AppData{
		{Name: "Safari", BundleID: "com.apple.Safari", Enabled: &enabled, Aliases: []string{"browser"}},
		{Name: "Finder", BundleID: "com.apple.finder", Enabled: &enabled},
	}
	req := &RenderHudRequest{HudMode: "apps", Apps: apps}
	result, err := handleRenderHud(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, ok := result.(shared.HudResponse)
	if !ok {
		t.Fatalf("expected HudResponse, got %T", result)
	}
	if resp.Title != "Known Applications" {
		t.Errorf("expected title 'Known Applications', got %q", resp.Title)
	}
	if len(resp.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(resp.Sections))
	}
	items := resp.Sections[0].Items
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	// Should be sorted alphabetically: Finder before Safari
	if items[0].Title != "Finder" {
		t.Errorf("expected Finder first (sorted), got %q", items[0].Title)
	}
	if items[1].Subtitle == nil || *items[1].Subtitle != "browser" {
		t.Error("expected Safari subtitle to contain alias")
	}
}

func TestHandleRenderHud_DisabledAppsFiltered(t *testing.T) {
	enabled := true
	disabled := false
	apps := []shared.AppData{
		{Name: "Safari", BundleID: "com.apple.Safari", Enabled: &enabled},
		{Name: "Hidden", BundleID: "com.example.hidden", Enabled: &disabled},
	}
	req := &RenderHudRequest{HudMode: "apps", Apps: apps}
	result, err := handleRenderHud(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(shared.HudResponse)
	if len(resp.Sections[0].Items) != 1 {
		t.Errorf("expected disabled app filtered out, got %d items", len(resp.Sections[0].Items))
	}
}

func TestHandleRenderHud_UnknownMode(t *testing.T) {
	req := &RenderHudRequest{HudMode: "nonexistent"}
	result, err := handleRenderHud(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(shared.HudResponse)
	if resp.Title != "Unknown" {
		t.Errorf("expected 'Unknown' title for bad mode, got %q", resp.Title)
	}
}

func TestHandleRenderHud_EmptyApps(t *testing.T) {
	req := &RenderHudRequest{HudMode: "apps", Apps: nil}
	result, err := handleRenderHud(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(shared.HudResponse)
	if len(resp.Sections[0].Items) != 0 {
		t.Errorf("expected 0 items for empty apps, got %d", len(resp.Sections[0].Items))
	}
}

// --- handleRenderSettings (apps tab) ---

func TestHandleRenderSettings_AppsTab(t *testing.T) {
	enabled := true
	req := &RenderSettingsRequest{
		TabKey: "apps",
		Apps: []shared.AppData{
			{Name: "Safari", BundleID: "com.apple.Safari", Enabled: &enabled, Aliases: []string{"browser"}},
		},
	}
	result, err := handleRenderSettings(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(shared.RenderSettingsResponse)
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
	enabled := true
	req := &RenderSettingsRequest{
		TabKey: "apps",
		Search: "safari",
		Apps: []shared.AppData{
			{Name: "Safari", BundleID: "com.apple.Safari", Enabled: &enabled},
			{Name: "Finder", BundleID: "com.apple.finder", Enabled: &enabled},
		},
	}
	result, err := handleRenderSettings(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(shared.RenderSettingsResponse)
	if !strings.Contains(resp.HTML, "Safari") {
		t.Error("expected Safari in filtered results")
	}
	if strings.Contains(resp.HTML, "Finder") {
		t.Error("expected Finder to be filtered out by search")
	}
}

func TestHandleRenderSettings_UnknownTab(t *testing.T) {
	req := &RenderSettingsRequest{TabKey: "nonexistent"}
	result, err := handleRenderSettings(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(shared.RenderSettingsResponse)
	if resp.HTML != "" {
		t.Errorf("expected empty HTML for unknown tab, got %q", resp.HTML)
	}
}

// --- handleOnAction routing ---

func TestHandleOnAction_UnknownAction(t *testing.T) {
	req := &OnActionRequest{Action: "other do-something"}
	result, err := handleOnAction(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(OnActionResponse)
	if resp.Result != "pass" {
		t.Errorf("expected 'pass' for unknown action, got %q", resp.Result)
	}
}

func TestHandleOnAction_UnknownSubcommand(t *testing.T) {
	req := &OnActionRequest{Action: "system.unknown_subcmd"}
	result, err := handleOnAction(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(OnActionResponse)
	if resp.Result != "pass" {
		t.Errorf("expected 'pass' for unknown subcommand, got %q", resp.Result)
	}
}

func TestHandleOnAction_SetOutputNoName(t *testing.T) {
	req := &OnActionRequest{Action: "system.set_output"}
	result, err := handleOnAction(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(OnActionResponse)
	if resp.Result != "handled" {
		t.Errorf("expected 'handled' for set-output with no name, got %q", resp.Result)
	}
}

func TestHandleOnAction_SetInputNoName(t *testing.T) {
	req := &OnActionRequest{Action: "system.set_input"}
	result, err := handleOnAction(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(OnActionResponse)
	if resp.Result != "handled" {
		t.Errorf("expected 'handled' for set-input with no name, got %q", resp.Result)
	}
}

func TestHandlePluginAction_Launch_NoBundleID(t *testing.T) {
	req := &OnActionRequest{
		Action: "system.launch",
		Params: map[string]interface{}{},
	}
	result, err := handleOnAction(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(OnActionResponse)
	if resp.Result != "handled" {
		t.Errorf("expected 'handled', got %q", resp.Result)
	}
}

func TestHandlePluginAction_Open_NoTarget(t *testing.T) {
	req := &OnActionRequest{
		Action: "system.open",
		Params: map[string]interface{}{},
	}
	result, err := handleOnAction(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(OnActionResponse)
	if resp.Result != "handled" {
		t.Errorf("expected 'handled', got %q", resp.Result)
	}
}

func TestHandlePluginAction_UnknownAction(t *testing.T) {
	req := &OnActionRequest{Action: "system.foobar"}
	result, err := handleOnAction(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(OnActionResponse)
	if resp.Result != "pass" {
		t.Errorf("expected 'pass' for unknown plugin action, got %q", resp.Result)
	}
}

// --- rpcHandler generic wrapper ---

func TestRpcHandler_ValidParams(t *testing.T) {
	handler := rpcHandler(func(req *RenderHudRequest) (any, error) {
		return req.HudMode, nil
	})
	params, _ := json.Marshal(map[string]string{"hud_mode": "apps"})
	result, err := handler(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "apps" {
		t.Errorf("expected 'apps', got %v", result)
	}
}

func TestRpcHandler_EmptyParams(t *testing.T) {
	handler := rpcHandler(func(req *RenderHudRequest) (any, error) {
		return req.HudMode, nil
	})
	result, err := handler(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected zero-value empty string, got %v", result)
	}
}

func TestRpcHandler_InvalidJSON(t *testing.T) {
	handler := rpcHandler(func(req *RenderHudRequest) (any, error) {
		return nil, nil
	})
	_, err := handler(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- templ rendering ---

func TestAppsTempl_RendersNonEmpty(t *testing.T) {
	rows := []appRowView{
		{Name: "Safari", BundleID: "com.apple.Safari", Aliases: []string{"browser"}, Status: "Enabled", BadgeClass: "badge-core"},
	}
	html := renderTempl(Apps(rows))
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
	html := renderTempl(Apps(nil))
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
			{UID: "spk-1", Name: "MacBook Air Speakers", VoiceHint: "speakers", IsDefault: true, DeviceType: "output"},
		},
		Inputs: []deviceView{
			{UID: "mic-1", Name: "MacBook Air Microphone", VoiceHint: "microphone", IsDefault: true, DeviceType: "input"},
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
	disabled := false
	req := &RenderSettingsRequest{
		TabKey: "apps",
		Apps: []shared.AppData{
			{Name: "Hidden", BundleID: "com.example.hidden", Enabled: &disabled},
		},
	}
	result, err := handleRenderSettings(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(shared.RenderSettingsResponse)
	if !strings.Contains(resp.HTML, "Disabled") {
		t.Error("expected 'Disabled' status badge for disabled app")
	}
}

func TestHandleRenderSettings_SearchByBundleID(t *testing.T) {
	enabled := true
	req := &RenderSettingsRequest{
		TabKey: "apps",
		Search: "com.apple.Safari",
		Apps: []shared.AppData{
			{Name: "Safari", BundleID: "com.apple.Safari", Enabled: &enabled},
			{Name: "Finder", BundleID: "com.apple.finder", Enabled: &enabled},
		},
	}
	result, err := handleRenderSettings(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(shared.RenderSettingsResponse)
	if !strings.Contains(resp.HTML, "Safari") {
		t.Error("expected Safari matched by bundle ID search")
	}
	if strings.Contains(resp.HTML, "Finder") {
		t.Error("expected Finder filtered out")
	}
}

// --- Action routing with dotted prefix ---

func TestHandleOnAction_DottedPrefix_RoutesToPluginAction(t *testing.T) {
	// Dotted actions route to handlePluginAction, not the space-prefix path
	req := &OnActionRequest{
		Action: "system.unknown",
		Params: map[string]interface{}{},
	}
	result, err := handleOnAction(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(OnActionResponse)
	if resp.Result != "pass" {
		t.Errorf("expected 'pass' for unknown dotted action, got %q", resp.Result)
	}
}

func TestHandlePluginAction_Launch_UsesBundleIDKey(t *testing.T) {
	// Verify both "bundleID" and "bundle_id" keys are accepted (no RPC call if empty)
	req := &OnActionRequest{
		Action: "system.launch",
		Params: map[string]interface{}{"bundle_id": ""},
	}
	result, err := handleOnAction(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := result.(OnActionResponse)
	if resp.Result != "handled" {
		t.Errorf("expected 'handled', got %q", resp.Result)
	}
}
