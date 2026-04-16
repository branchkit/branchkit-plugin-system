package main

import (
	"bytes"
	"context"
	"sort"
	"strings"

	"github.com/a-h/templ"
	"github.com/branchkit/plugin-sdk-go"
)

// --- Local view types (request/response types come from shared) ---

type RenderHudRequest struct {
	HudMode string           `json:"hud_mode"`
	Apps    []shared.AppData `json:"apps"`
}

type RenderSettingsRequest struct {
	TabKey string           `json:"tab_key"`
	Search string           `json:"search"`
	Apps   []shared.AppData `json:"apps"`
}

// jsEscape escapes a string for safe use in single-quoted JavaScript literals.
func jsEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

// renderTempl renders a templ component to an HTML string.
func renderTempl(c templ.Component) string {
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		shared.Logf("system", "templ render error: %v", err)
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

var plugin *shared.Plugin

func handleRenderHud(req *RenderHudRequest) (any, error) {
	if req.HudMode != "apps" {
		return shared.HudResponse{
			Title:    "Unknown",
			Sections: []shared.HudSection{},
		}, nil
	}

	items := []shared.HudItem{}
	for _, app := range req.Apps {
		if !app.IsEnabled() {
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

func handleRenderSettings(req *RenderSettingsRequest) (any, error) {
	if req.TabKey == "sound" {
		html := renderSoundSettings(plugin)
		return shared.RenderSettingsResponse{HTML: html}, nil
	}

	if req.TabKey != "apps" {
		return shared.RenderSettingsResponse{}, nil
	}

	search := strings.ToLower(req.Search)
	var rows []appRowView
	for _, app := range req.Apps {
		if search != "" &&
			!strings.Contains(strings.ToLower(app.Name), search) &&
			!strings.Contains(strings.ToLower(app.BundleID), search) {
			continue
		}
		status := "Enabled"
		badgeClass := "badge-core"
		if !app.IsEnabled() {
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

	return shared.RenderSettingsResponse{HTML: renderTempl(Apps(rows))}, nil
}

// --- Per-action handlers ---

func handleVolumeUp(_ *shared.OnActionRequest) (any, error) {
	if err := volumeUp(); err != nil {
		shared.Logf("system", "volume_up: %v", err)
	}
	return nil, nil
}

func handleVolumeDown(_ *shared.OnActionRequest) (any, error) {
	if err := volumeDown(); err != nil {
		shared.Logf("system", "volume_down: %v", err)
	}
	return nil, nil
}

func handleMute(_ *shared.OnActionRequest) (any, error) {
	if err := mute(); err != nil {
		shared.Logf("system", "mute: %v", err)
	}
	return nil, nil
}

func handleUnmute(_ *shared.OnActionRequest) (any, error) {
	if err := unmute(); err != nil {
		shared.Logf("system", "unmute: %v", err)
	}
	return nil, nil
}

type setDeviceParams struct {
	Name string `json:"name"`
}

func handleSetOutput(req *shared.OnActionRequest) (any, error) {
	var p setDeviceParams
	if err := req.UnmarshalParams(&p); err != nil {
		return nil, err
	}
	if p.Name == "" {
		shared.Logf("system", "set_output: no device name provided")
		return nil, nil
	}
	if err := setOutputDevice(plugin, p.Name); err != nil {
		shared.Logf("system", "set_output: %v", err)
	}
	return nil, nil
}

func handleSetInput(req *shared.OnActionRequest) (any, error) {
	var p setDeviceParams
	if err := req.UnmarshalParams(&p); err != nil {
		return nil, err
	}
	if p.Name == "" {
		shared.Logf("system", "set_input: no device name provided")
		return nil, nil
	}
	if err := setInputDevice(plugin, p.Name); err != nil {
		shared.Logf("system", "set_input: %v", err)
	}
	return nil, nil
}

type launchParams struct {
	BundleID    string `json:"bundleID"`
	BundleIDAlt string `json:"bundle_id"`
	NewInstance bool   `json:"new_instance"`
}

func handleLaunch(req *shared.OnActionRequest) (any, error) {
	var p launchParams
	if err := req.UnmarshalParams(&p); err != nil {
		return nil, err
	}
	bundleID := p.BundleID
	if bundleID == "" {
		bundleID = p.BundleIDAlt
	}
	if bundleID == "" {
		shared.Logf("system", "launch: no bundleID provided")
		return nil, nil
	}
	if err := plugin.Call("native.launch_app", map[string]any{
		"bundle_id":    bundleID,
		"new_instance": p.NewInstance,
	}, nil); err != nil {
		shared.Logf("system", "launch: %v", err)
	}
	return nil, nil
}

type openParams struct {
	Target string `json:"target"`
}

func handleOpen(req *shared.OnActionRequest) (any, error) {
	var p openParams
	if err := req.UnmarshalParams(&p); err != nil {
		return nil, err
	}
	if p.Target == "" {
		shared.Logf("system", "open: no target provided")
		return nil, nil
	}
	if err := plugin.Call("native.open_target", map[string]any{
		"target": p.Target,
	}, nil); err != nil {
		shared.Logf("system", "open: %v", err)
	}
	return nil, nil
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

func pushCommands(p *shared.Plugin) {
	count, err := shared.PushCommands(p)
	if err != nil {
		shared.Logf("system", "%v", err)
		return
	}
	shared.Logf("system", "Registered %d command variants", count)
}

func main() {
	plugin = shared.NewPlugin()
	initDeviceAliases()
	pushCommands(plugin)

	// Per-action handlers (replaces the old single on_action switch).
	plugin.HandleAction("system.volume_up", handleVolumeUp)
	plugin.HandleAction("system.volume_down", handleVolumeDown)
	plugin.HandleAction("system.mute", handleMute)
	plugin.HandleAction("system.unmute", handleUnmute)
	plugin.HandleAction("system.set_output", handleSetOutput)
	plugin.HandleAction("system.set_input", handleSetInput)
	plugin.HandleAction("system.launch", handleLaunch)
	plugin.HandleAction("system.open", handleOpen)

	shared.HandleTyped(plugin, "render_hud", handleRenderHud)
	shared.HandleTyped(plugin, "render_settings", handleRenderSettings)
	shared.HandleTyped(plugin, "set_volume", handleSetVolume)
	shared.HandleTyped(plugin, "set_mute", handleSetMute)
	shared.HandleTyped(plugin, "set_device", handleSetDevice)
	shared.HandleTyped(plugin, "device_alias_add", handleDeviceAliasAdd)
	shared.HandleTyped(plugin, "device_alias_remove", handleDeviceAliasRemove)

	plugin.Run()
}
