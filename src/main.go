package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"sort"
	"strings"

	"github.com/branchkit/plugin-sdk-go"
)

//go:embed templates/apps.html
var appsTemplateHTML string

// --- Request/Response types ---

type OnActionRequest struct {
	Action         string   `json:"action"`
	Args           []string `json:"args,omitempty"`
	ActiveApp      *string  `json:"active_app,omitempty"`
	ActiveWindowID *string  `json:"active_window_id,omitempty"`
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

// --- Templates ---

var appsSettingsTemplate = template.Must(template.New("apps").Funcs(template.FuncMap{
	"jsEscape": func(s string) string {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `'`, `\'`)
		return s
	},
}).Parse(appsTemplateHTML))

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

	var buf bytes.Buffer
	if err := appsSettingsTemplate.Execute(&buf, struct{ Apps []appRowView }{Apps: rows}); err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] template error: %v\n", err)
		return shared.RenderSettingsResponse{}, nil
	}
	return shared.RenderSettingsResponse{HTML: buf.String()}, nil
}

func handleOnAction(req *OnActionRequest) (any, error) {
	sub, ok := strings.CutPrefix(req.Action, "system ")
	if !ok {
		return OnActionResponse{Result: "pass"}, nil
	}

	// Split sub-command from inline args (action string includes args, e.g. "set-output speakers")
	subCmd, argStr, _ := strings.Cut(sub, " ")
	// Merge inline args with explicit Args field (prefer explicit if present)
	args := req.Args
	if len(args) == 0 && argStr != "" {
		args = strings.Fields(argStr)
	}

	var err error
	switch subCmd {
	case "volume-up":
		err = volumeUp()
	case "volume-down":
		err = volumeDown()
	case "mute":
		err = mute()
	case "unmute":
		err = unmute()
	case "set-output":
		name := strings.Join(args, " ")
		if name == "" {
			fmt.Fprintf(os.Stderr, "[SYSTEM] set-output: no device name provided\n")
			return OnActionResponse{Result: "handled"}, nil
		}
		err = setOutputDevice(plugin, name)
	case "set-input":
		name := strings.Join(args, " ")
		if name == "" {
			fmt.Fprintf(os.Stderr, "[SYSTEM] set-input: no device name provided\n")
			return OnActionResponse{Result: "handled"}, nil
		}
		err = setInputDevice(plugin, name)
	default:
		return OnActionResponse{Result: "pass"}, nil
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] audio %s error: %v\n", sub, err)
	}
	return OnActionResponse{Result: "handled"}, nil
}

// --- Sound settings hook handlers ---

type setVolumeRequest struct {
	Volume int `json:"volume"`
}

func handleSetVolume(req *setVolumeRequest) (any, error) {
	if err := setVolume(req.Volume); err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] set-volume error: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "[SYSTEM] set-mute error: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "[SYSTEM] set-device error: %v\n", err)
	}
	return map[string]string{"result": "ok"}, nil
}

func pushCommands(p *shared.Plugin) {
	pluginDir := os.Getenv("BRANCHKIT_PLUGIN_DIR")
	if pluginDir == "" {
		return
	}
	data, err := os.ReadFile(pluginDir + "/commands.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] failed to read commands.json: %v\n", err)
		return
	}
	var commands []json.RawMessage
	if err := json.Unmarshal(data, &commands); err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] failed to parse commands.json: %v\n", err)
		return
	}
	var resp struct{ Count int `json:"count"` }
	if err := p.Call("grammar.push", map[string]any{"commands": commands}, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] grammar.push failed: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "[SYSTEM] Registered %d command variants\n", resp.Count)
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
