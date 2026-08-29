package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/a-h/templ"
	"github.com/branchkit/plugin-sdk-go"
)

// jsEscape escapes a string for safe embedding in JavaScript string literals
// (single-quoted or double-quoted), for use in templ files. Handles
// backslashes, quotes, and newlines.
func jsEscape(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch c {
		case '\\':
			b.WriteString("\\\\")
		case '\'':
			b.WriteString("\\'")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// renderTempl renders a templ component to an HTML string.
func renderTempl(c templ.Component) string {
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		branchkit.Logf("system", "templ render error: %v", err)
		return ""
	}
	return buf.String()
}

type appRowView struct {
	Name       string
	BundleID   string
	Aliases    []string
	Status     string
	BadgeClass string
}

// --- Handlers ---

var plugin *branchkit.Plugin

func handleRenderSettings(req *branchkit.RenderSettingsRequest) (any, error) {
	if req.TabKey == "sound" {
		html := renderSoundSettings(plugin)
		return branchkit.RenderSettingsResponse{HTML: html}, nil
	}

	if req.TabKey == "devices" {
		html := renderDevicesSettings(plugin)
		return branchkit.RenderSettingsResponse{HTML: html}, nil
	}

	if req.TabKey != "apps" {
		return branchkit.RenderSettingsResponse{}, nil
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

	// Read through, not just the cache: this render may be the one fired
	// right after the mouse-follows-focus toggle's own write.
	conf := LoadSystemConfig()
	if configMirror != nil {
		if fresh, err := configMirror.Load(); err == nil {
			conf = fresh
		} else {
			branchkit.Logf("system", "config read-through failed: %v", err)
		}
	}
	return branchkit.RenderSettingsResponse{HTML: renderTempl(Apps(rows, conf.MouseFollowsFocus))}, nil
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
		branchkit.Logf("system", "config relay error: %v", err)
	}
	return map[string]string{"result": "ok"}, nil
}

// --- Sound settings hook handlers ---

type setVolumeRequest struct {
	Volume int `json:"volume"`
}

func handleSetVolume(req *setVolumeRequest) (any, error) {
	if err := setVolume(float64(req.Volume) / 100.0); err != nil {
		branchkit.Logf("system", "set-volume error: %v", err)
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
		branchkit.Logf("system", "set-mute error: %v", err)
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
		branchkit.Logf("system", "set-device error: %v", err)
	}
	return map[string]string{"result": "ok"}, nil
}

// --- Startup ---

func main() {
	plugin = branchkit.NewPlugin()
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

	branchkit.HandleTyped(plugin, "render_settings", handleRenderSettings)
	branchkit.HandleTyped(plugin, "set_volume", handleSetVolume)
	branchkit.HandleTyped(plugin, "set_mute", handleSetMute)
	branchkit.HandleTyped(plugin, "set_device", handleSetDevice)
	branchkit.HandleTyped(plugin, "device_alias_add", handleDeviceAliasAdd)
	branchkit.HandleTyped(plugin, "device_alias_remove", handleDeviceAliasRemove)
	branchkit.HandleTyped(plugin, "app_toggle", handleAppToggle)
	branchkit.HandleTyped(plugin, "app_alias_add", handleAppAliasAdd)
	branchkit.HandleTyped(plugin, "app_alias_remove", handleAppAliasRemove)
	branchkit.HandleTyped(plugin, "set_mouse_follows_focus", handleSetMouseFollowsFocus)

	// Publish current audio device names as speakable collections once RPC is
	// available (OnReady), so "set output/input <device>" matches real names.
	// Re-push on hotplug so the collections track the live device set — the
	// replace is idempotent (byte-identical records are skipped platform-
	// side), so bursts and no-op
	// changes (e.g. default-device moves) don't churn the grammar.
	plugin.OnReady(func() { pushAudioDevicesCollections(plugin) })
	plugin.On("_platform.audio_devices.changed", func(json.RawMessage) {
		pushAudioDevicesCollections(plugin)
	})
	// Devices can come and go while the machine is asleep — a dock unplugged,
	// Bluetooth headphones taken out of range — and the CoreAudio property
	// listener that feeds audio_devices.changed is not running to see it. Wake
	// is the one moment the collections are guaranteed stale, so re-push.
	// Same idempotent replace as hotplug: no churn when nothing moved.
	plugin.On("_platform.system.did_wake", func(json.RawMessage) {
		pushAudioDevicesCollections(plugin)
	})

	plugin.Run()
}
