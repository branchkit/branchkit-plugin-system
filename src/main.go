package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/a-h/templ"
	"github.com/branchkit/plugin-sdk-go"
)

// --- Request/Response types ---

type OnActionRequest struct {
	Action         string                 `json:"action"`
	Args           []string               `json:"args,omitempty"`
	Params         map[string]interface{} `json:"params,omitempty"`
	ActiveApp      *string                `json:"active_app,omitempty"`
	ActiveWindowID *string                `json:"active_window_id,omitempty"`
}

type OnActionResponse struct {
	Result string `json:"result"`
}

type RenderHudRequest struct {
	HudMode string         `json:"hud_mode"`
	Apps    []shared.AppData `json:"apps"`
}

type RenderSettingsRequest struct {
	TabKey string         `json:"tab_key"`
	Search string         `json:"search"`
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

// --- Generic RPC handler ---

func rpcHandler[Req any](fn func(*Req) (any, error)) shared.HandlerFunc {
	return func(params json.RawMessage) (any, error) {
		var req Req
		if len(params) > 0 {
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, fmt.Errorf("bad params: %w", err)
			}
		}
		return fn(&req)
	}
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

func handleOnAction(req *OnActionRequest) (any, error) {
	p := req.Params
	args := req.Args

	var err error
	switch req.Action {
	case "system.volume_up":
		err = volumeUp()
	case "system.volume_down":
		err = volumeDown()
	case "system.mute":
		err = mute()
	case "system.unmute":
		err = unmute()
	case "system.set_output":
		name := strings.Join(args, " ")
		if name == "" {
			shared.Logf("system", "set_output: no device name provided")
			return OnActionResponse{Result: "handled"}, nil
		}
		err = setOutputDevice(plugin, name)
	case "system.set_input":
		name := strings.Join(args, " ")
		if name == "" {
			shared.Logf("system", "set_input: no device name provided")
			return OnActionResponse{Result: "handled"}, nil
		}
		err = setInputDevice(plugin, name)
	case "system.launch":
		bundleID, _ := p["bundleID"].(string)
		if bundleID == "" {
			bundleID, _ = p["bundle_id"].(string)
		}
		if bundleID == "" {
			shared.Logf("system", "launch: no bundleID provided")
			return OnActionResponse{Result: "handled"}, nil
		}
		newInstance, _ := p["new_instance"].(bool)
		err = plugin.Call("native.launch_app", map[string]interface{}{
			"bundle_id":    bundleID,
			"new_instance": newInstance,
		}, nil)
	case "system.open":
		target, _ := p["target"].(string)
		if target == "" {
			shared.Logf("system", "open: no target provided")
			return OnActionResponse{Result: "handled"}, nil
		}
		err = plugin.Call("native.open_target", map[string]interface{}{
			"target": target,
		}, nil)
	default:
		return OnActionResponse{Result: "pass"}, nil
	}

	if err != nil {
		shared.Logf("system", "on_action %s error: %v", req.Action, err)
	}
	return OnActionResponse{Result: "handled"}, nil
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

	// Register handlers (actuator→plugin requests)
	plugin.Handle("on_action", rpcHandler(handleOnAction))
	plugin.Handle("render_hud", rpcHandler(handleRenderHud))
	plugin.Handle("render_settings", rpcHandler(handleRenderSettings))
	plugin.Handle("set_volume", rpcHandler(handleSetVolume))
	plugin.Handle("set_mute", rpcHandler(handleSetMute))
	plugin.Handle("set_device", rpcHandler(handleSetDevice))
	plugin.Handle("device_alias_add", rpcHandler(handleDeviceAliasAdd))
	plugin.Handle("device_alias_remove", rpcHandler(handleDeviceAliasRemove))

	// Run the message loop (blocks until stdin closes or SIGTERM)
	plugin.Run()
}
