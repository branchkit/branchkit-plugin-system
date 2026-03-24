package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"branchkit.local/shared"
)

//go:embed templates/apps.html
var appsTemplateHTML string

// --- On-action types ---

type OnActionRequest struct {
	Action         string   `json:"action"`
	Args           []string `json:"args,omitempty"`
	ActiveApp      *string  `json:"active_app,omitempty"`
	ActiveWindowID *string  `json:"active_window_id,omitempty"`
}

type OnActionResponse struct {
	Result string `json:"result"`
}

// --- Request/Response types ---

type AppData struct {
	Name     string   `json:"name"`
	BundleID string   `json:"bundle_id"`
	Aliases  []string `json:"aliases"`
	Enabled  *bool    `json:"enabled"` // pointer to detect missing (defaults to true)
}

func (a AppData) IsEnabled() bool {
	if a.Enabled == nil {
		return true
	}
	return *a.Enabled
}

type RenderHudRequest struct {
	HudMode string    `json:"hud_mode"`
	Apps    []AppData `json:"apps"`
}

type RenderSettingsRequest struct {
	TabKey string    `json:"tab_key"`
	Search string    `json:"search"`
	Apps   []AppData `json:"apps"`
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

// --- Handlers ---

func handleHealth(w http.ResponseWriter, r *http.Request) {
	shared.WriteJSON(w, map[string]bool{"ready": true})
}

func handleRenderHud(w http.ResponseWriter, r *http.Request) {
	var req RenderHudRequest
	if err := shared.ReadJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.HudMode != "apps" {
		shared.WriteJSON(w, shared.HudResponse{
			Title:    "Unknown",
			Sections: []shared.HudSection{},
		})
		return
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

	shared.WriteJSON(w, shared.HudResponse{
		Title:  "Known Applications",
		Footer: "Say 'switch <name>' or 'open <name>'",
		Sections: []shared.HudSection{
			{Title: "Applications", Items: items},
		},
	})
}

func handleRenderSettings(w http.ResponseWriter, r *http.Request) {
	var req RenderSettingsRequest
	if err := shared.ReadJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.TabKey == "sound" {
		html := renderSoundSettings(platform)
		shared.WriteJSON(w, shared.SettingsResponse{HTML: html})
		return
	}

	if req.TabKey != "apps" {
		shared.WriteJSON(w, shared.SettingsResponse{})
		return
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
		shared.WriteJSON(w, shared.SettingsResponse{})
		return
	}
	shared.WriteJSON(w, shared.SettingsResponse{HTML: buf.String()})
}

var platform *shared.PlatformClient

func handleOnAction(w http.ResponseWriter, r *http.Request) {
	var req OnActionRequest
	if err := shared.ReadJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sub, ok := strings.CutPrefix(req.Action, "system ")
	if !ok {
		shared.WriteJSON(w, OnActionResponse{Result: "pass"})
		return
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
			shared.WriteJSON(w, OnActionResponse{Result: "handled"})
			return
		}
		err = setOutputDevice(platform, name)
	case "set-input":
		name := strings.Join(args, " ")
		if name == "" {
			fmt.Fprintf(os.Stderr, "[SYSTEM] set-input: no device name provided\n")
			shared.WriteJSON(w, OnActionResponse{Result: "handled"})
			return
		}
		err = setInputDevice(platform, name)
	default:
		shared.WriteJSON(w, OnActionResponse{Result: "pass"})
		return
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] audio %s error: %v\n", sub, err)
	}
	shared.WriteJSON(w, OnActionResponse{Result: "handled"})
}

// --- Sound settings hook handlers ---

func handleSetVolume(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var payload struct {
		Volume int `json:"volume"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := setVolume(payload.Volume); err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] set-volume error: %v\n", err)
	}
	shared.WriteJSON(w, map[string]string{"result": "ok"})
}

func handleSetMute(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var payload struct {
		Muted bool `json:"muted"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if payload.Muted {
		err = mute()
	} else {
		err = unmute()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] set-mute error: %v\n", err)
	}
	shared.WriteJSON(w, map[string]string{"result": "ok"})
}

func handleDeviceAliasAdd(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var payload struct {
		UID   string `json:"uid"`
		Alias string `json:"newAlias"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	addDeviceAlias(payload.UID, payload.Alias)
	shared.WriteJSON(w, map[string]string{"result": "ok"})
}

func handleDeviceAliasRemove(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var payload struct {
		UID   string `json:"uid"`
		Alias string `json:"newAlias"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	removeDeviceAlias(payload.UID, payload.Alias)
	shared.WriteJSON(w, map[string]string{"result": "ok"})
}

func handleSetDevice(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var payload struct {
		UID        string `json:"uid"`
		DeviceType string `json:"device_type"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := platform.SetAudioDevice(payload.UID, payload.DeviceType); err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] set-device error: %v\n", err)
	}
	shared.WriteJSON(w, map[string]string{"result": "ok"})
}

func main() {
	platform = shared.NewPlatformClient()
	initDeviceAliases()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("POST /hooks/on-action", handleOnAction)
	mux.HandleFunc("POST /hooks/render-hud", handleRenderHud)
	mux.HandleFunc("POST /hooks/render-settings", handleRenderSettings)
	mux.HandleFunc("POST /hooks/set-volume", handleSetVolume)
	mux.HandleFunc("POST /hooks/set-mute", handleSetMute)
	mux.HandleFunc("POST /hooks/set-device", handleSetDevice)
	mux.HandleFunc("POST /hooks/device-alias-add", handleDeviceAliasAdd)
	mux.HandleFunc("POST /hooks/device-alias-remove", handleDeviceAliasRemove)

	shared.RunPlugin(mux)
}
