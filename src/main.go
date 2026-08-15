package main

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/a-h/templ"
	"github.com/branchkit/plugin-sdk-go"
	toolkit "github.com/branchkit/plugin-toolkit-go"
)

// --- Local view types (request/response types come from shared) ---

type RenderHudRequest struct {
	HudMode string `json:"hud_mode"`
}

// jsEscape wraps toolkit.JSEscape for use in templ files.
func jsEscape(s string) string { return toolkit.JSEscape(s) }

// renderTempl renders a templ component to an HTML string.
func renderTempl(c templ.Component) string {
	return toolkit.RenderTempl("system", c)
}

type appRowView struct {
	Name       string
	BundleID   string
	Aliases    []string
	Status     string
	BadgeClass string
}

// --- Handlers ---

var plugin *shared.Plugin

func handleRenderHud(req *RenderHudRequest) (any, error) {
	if req.HudMode != "apps" {
		return shared.HudResponse{
			Title:    "Unknown",
			Sections: []shared.HudSection{},
		}, nil
	}

	allApps := getApps()
	items := []shared.HudItem{}
	for _, app := range allApps {
		if !app.Enabled {
			continue
		}
		var subtitle *string
		if len(app.Aliases) > 0 {
			s := strings.Join(app.Aliases, ", ")
			subtitle = &s
		}
		items = append(items, shared.HudItem{
			ID:       app.BundleID,
			Title:    app.Name,
			Subtitle: subtitle,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
	})

	return shared.HudResponse{
		Title:  "Known Applications",
		Footer: "Say 'switch <name>' or 'open <name>'",
		Sections: []shared.HudSection{
			{Title: "Applications", Items: items},
		},
	}, nil
}

func handleRenderSettings(req *shared.RenderSettingsRequest) (any, error) {
	if req.TabKey == "sound" {
		html := renderSoundSettings(plugin)
		return shared.RenderSettingsResponse{HTML: html}, nil
	}

	if req.TabKey == "devices" {
		html := renderDevicesSettings(plugin)
		return shared.RenderSettingsResponse{HTML: html}, nil
	}

	if req.TabKey != "apps" {
		return shared.RenderSettingsResponse{}, nil
	}

	allApps := getApps()
	search := strings.ToLower(req.Search)
	var rows []appRowView
	for _, app := range allApps {
		if search != "" &&
			!strings.Contains(strings.ToLower(app.Name), search) &&
			!strings.Contains(strings.ToLower(app.BundleID), search) {
			continue
		}
		status := "Enabled"
		badgeClass := "badge-core"
		if !app.Enabled {
			status = "Disabled"
			badgeClass = "badge-user"
		}
		rows = append(rows, appRowView{
			Name:       app.Name,
			BundleID:   app.BundleID,
			Aliases:    app.Aliases,
			Status:     status,
			BadgeClass: badgeClass,
		})
	}

	return shared.RenderSettingsResponse{HTML: renderTempl(Apps(rows, LoadSystemConfig().MouseFollowsFocus))}, nil
}

// --- App settings action handlers ---

type appToggleRequest struct {
	BundleID string `json:"bundle_id"`
}

func handleAppToggle(req *appToggleRequest) (any, error) {
	toggleApp(plugin, req.BundleID)
	return map[string]string{"result": "ok"}, nil
}

type appAliasRequest struct {
	BundleID string `json:"bundle_id"`
	Alias    string `json:"newAlias"`
}

func handleAppAliasAdd(req *appAliasRequest) (any, error) {
	addAppAlias(plugin, req.BundleID, req.Alias)
	return map[string]string{"result": "ok"}, nil
}

func handleAppAliasRemove(req *appAliasRequest) (any, error) {
	removeAppAlias(plugin, req.BundleID, req.Alias)
	return map[string]string{"result": "ok"}, nil
}

type setMouseFollowsFocusRequest struct {
	Enabled bool `json:"enabled"`
}

func handleSetMouseFollowsFocus(req *setMouseFollowsFocusRequest) (any, error) {
	if err := setUserConfigField("mouse_follows_focus", req.Enabled); err != nil {
		shared.Logf("system", "config relay error: %v", err)
	}
	return map[string]string{"result": "ok"}, nil
}

// --- Sound settings hook handlers ---

type setVolumeRequest struct {
	Volume int `json:"volume"`
}

func handleSetVolume(req *setVolumeRequest) (any, error) {
	if err := setVolume(float64(req.Volume) / 100.0); err != nil {
		shared.Logf("system", "set-volume error: %v", err)
	}
	return map[string]string{"result": "ok"}, nil
}

type setMuteRequest struct {
	Muted bool `json:"muted"`
}

func handleSetMute(req *setMuteRequest) (any, error) {
	var err error
	if req.Muted {
		err = mute()
	} else {
		err = unmute()
	}
	if err != nil {
		shared.Logf("system", "set-mute error: %v", err)
	}
	return map[string]string{"result": "ok"}, nil
}

type deviceAliasRequest struct {
	UID   string `json:"uid"`
	Alias string `json:"newAlias"`
}

func handleDeviceAliasAdd(req *deviceAliasRequest) (any, error) {
	addDeviceAlias(req.UID, req.Alias)
	return map[string]string{"result": "ok"}, nil
}

func handleDeviceAliasRemove(req *deviceAliasRequest) (any, error) {
	removeDeviceAlias(req.UID, req.Alias)
	return map[string]string{"result": "ok"}, nil
}

type setDeviceRequest struct {
	UID        string `json:"uid"`
	DeviceType string `json:"device_type"`
}

func handleSetDevice(req *setDeviceRequest) (any, error) {
	if err := setAudioDeviceViaRPC(plugin, req.UID, req.DeviceType); err != nil {
		shared.Logf("system", "set-device error: %v", err)
	}
	return map[string]string{"result": "ok"}, nil
}

// --- Startup ---

func main() {
	plugin = shared.NewPlugin()
	initApps(plugin)
	initConfig(plugin)

	// Per-action handlers. Registrars come from actions_gen.go, generated from
	// plugin.json — so no action string is spelled here and a handler's params
	// type cannot drift from what the manifest declares.
	HandleVolumeUp(plugin, handleVolumeUp)
	HandleVolumeDown(plugin, handleVolumeDown)
	HandleMute(plugin, handleMute)
	HandleUnmute(plugin, handleUnmute)
	HandleSetOutput(plugin, handleSetOutput)
	HandleSetInput(plugin, handleSetInput)
	HandleLaunch(plugin, handleLaunch)
	HandleNewWindow(plugin, handleNewWindow)
	HandleOpen(plugin, handleOpen)

	shared.HandleTyped(plugin, "render_hud", handleRenderHud)
	shared.HandleTyped(plugin, "render_settings", handleRenderSettings)
	shared.HandleTyped(plugin, "set_volume", handleSetVolume)
	shared.HandleTyped(plugin, "set_mute", handleSetMute)
	shared.HandleTyped(plugin, "set_device", handleSetDevice)
	shared.HandleTyped(plugin, "device_alias_add", handleDeviceAliasAdd)
	shared.HandleTyped(plugin, "device_alias_remove", handleDeviceAliasRemove)
	shared.HandleTyped(plugin, "app_toggle", handleAppToggle)
	shared.HandleTyped(plugin, "app_alias_add", handleAppAliasAdd)
	shared.HandleTyped(plugin, "app_alias_remove", handleAppAliasRemove)
	shared.HandleTyped(plugin, "set_mouse_follows_focus", handleSetMouseFollowsFocus)

	// Publish current audio device names as speakable collections once RPC is
	// available (OnReady), so "set output/input <device>" matches real names.
	// Re-push on hotplug so the collections track the live device set — the
	// push is idempotent (ReplaceCollection diffs), so bursts and no-op
	// changes (e.g. default-device moves) don't churn the grammar.
	plugin.OnReady(func() { pushAudioDevicesCollections(plugin) })
	plugin.On("_platform.audio_devices.changed", func(json.RawMessage) {
		pushAudioDevicesCollections(plugin)
	})

	plugin.Run()
}
