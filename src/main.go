package main

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/branchkit/plugin-sdk-go"
)

//go:embed settings.css
var systemCSS string

type appRowView struct {
	Name       string
	BundleID   string
	Aliases    []string
	Traits     []string
	Status     string
	BadgeClass string
}

// --- Handlers ---

func (h *Host) renderSoundTab(_ *branchkit.RenderSettingsRequest) (string, error) {
	return h.renderSoundSettings(h.plugin)
}

func (h *Host) renderDevicesTab(_ *branchkit.RenderSettingsRequest) (string, error) {
	return renderDevicesSettings(h.plugin)
}

func (h *Host) renderAppsTab(req *branchkit.RenderSettingsRequest) (string, error) {
	allApps := h.getApps()

	traits := loadTraitCatalog(h.plugin)
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
			Traits:     traits.ByBundle[app.BundleID],
			Status:     status,
			BadgeClass: badgeClass,
		})
	}

	// The SDK's render hook has already refreshed the config mirror, so
	// this read is current even when the render is the one fired right
	// after the mouse-follows-focus toggle's own write.
	conf := h.LoadSystemConfig()
	return branchkit.RenderComponent(Apps(rows, conf.MouseFollowsFocus, traits))
}

// --- App settings action handlers ---

type appToggleRequest struct {
	BundleID string `json:"bundle_id"`
}

func (h *Host) handleAppToggle(req *appToggleRequest) (any, error) {
	h.toggleApp(h.plugin, req.BundleID)
	return nil, nil
}

type appAliasRequest struct {
	BundleID string `json:"bundle_id"`
	Alias    string `json:"newAlias"`
}

func (h *Host) handleAppAliasAdd(req *appAliasRequest) (any, error) {
	h.addAppAlias(h.plugin, req.BundleID, req.Alias)
	return nil, nil
}

func (h *Host) handleAppAliasRemove(req *appAliasRequest) (any, error) {
	h.removeAppAlias(h.plugin, req.BundleID, req.Alias)
	return nil, nil
}

// appTraitRequest carries one trait edit from the Apps settings tab.
type appTraitRequest struct {
	BundleID string `json:"bundle_id"`
	Trait    string `json:"trait"`
}

// The trait handlers RETURN their error, unlike their alias counterparts which
// log and report success. A rejected trait name is something the user just
// typed and needs to see; an alias add can only fail on a transport error
// nobody could act on.
func (h *Host) handleAppTraitAdd(req *appTraitRequest) (any, error) {
	if err := addAppTrait(h.plugin, req.BundleID, req.Trait); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *Host) handleAppTraitRemove(req *appTraitRequest) (any, error) {
	if err := removeAppTrait(h.plugin, req.BundleID, req.Trait); err != nil {
		return nil, err
	}
	return nil, nil
}

type setMouseFollowsFocusRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *Host) handleSetMouseFollowsFocus(req *setMouseFollowsFocusRequest) (any, error) {
	if err := h.setUserConfigField("mouse_follows_focus", req.Enabled); err != nil {
		branchkit.Logf("system", "config relay error: %v", err)
	}
	return nil, nil
}

// --- Sound settings hook handlers ---

type setVolumeRequest struct {
	Volume int `json:"volume"`
}

func (h *Host) handleSetVolume(req *setVolumeRequest) (any, error) {
	if err := h.setVolume(float64(req.Volume) / 100.0); err != nil {
		branchkit.Logf("system", "set-volume error: %v", err)
	}
	return nil, nil
}

type setMuteRequest struct {
	Muted bool `json:"muted"`
}

func (h *Host) handleSetMute(req *setMuteRequest) (any, error) {
	var err error
	if req.Muted {
		err = h.mute()
	} else {
		err = h.unmute()
	}
	if err != nil {
		branchkit.Logf("system", "set-mute error: %v", err)
	}
	return nil, nil
}

type deviceAliasRequest struct {
	UID   string `json:"uid"`
	Alias string `json:"newAlias"`
}

func (h *Host) handleDeviceAliasAdd(req *deviceAliasRequest) (any, error) {
	h.addDeviceAlias(req.UID, req.Alias)
	return nil, nil
}

func (h *Host) handleDeviceAliasRemove(req *deviceAliasRequest) (any, error) {
	h.removeDeviceAlias(req.UID, req.Alias)
	return nil, nil
}

type setDeviceRequest struct {
	UID        string `json:"uid"`
	DeviceType string `json:"device_type"`
}

func (h *Host) handleSetDevice(req *setDeviceRequest) (any, error) {
	if err := setAudioDeviceViaRPC(h.plugin, req.UID, req.DeviceType); err != nil {
		branchkit.Logf("system", "set-device error: %v", err)
	}
	return nil, nil
}

// --- Startup ---

func main() {
	h := newHost(branchkit.NewPlugin())
	h.initApps(h.plugin)
	h.initConfig(h.plugin)

	// Per-action handlers. Registrars come from actions_gen.go, generated from
	// plugin.json — so no action string is spelled here and a handler's params
	// type cannot drift from what the manifest declares.
	HandleVolumeUp(h.plugin, h.handleVolumeUp)
	HandleVolumeDown(h.plugin, h.handleVolumeDown)
	HandleMute(h.plugin, h.handleMute)
	HandleUnmute(h.plugin, h.handleUnmute)
	HandleSetOutput(h.plugin, h.handleSetOutput)
	HandleSetInput(h.plugin, h.handleSetInput)
	HandleLaunch(h.plugin, h.handleLaunch)
	HandleNewWindow(h.plugin, h.handleNewWindow)
	HandleOpen(h.plugin, h.handleOpen)

	h.plugin.SettingsCSS(systemCSS)
	h.plugin.SettingsTab("apps", h.renderAppsTab)
	h.plugin.SettingsTab("sound", h.renderSoundTab)
	h.plugin.SettingsTab("devices", h.renderDevicesTab)

	branchkit.HandleTyped(h.plugin, "set_volume", h.handleSetVolume)
	branchkit.HandleTyped(h.plugin, "set_mute", h.handleSetMute)
	branchkit.HandleTyped(h.plugin, "set_device", h.handleSetDevice)
	branchkit.HandleTyped(h.plugin, "device_alias_add", h.handleDeviceAliasAdd)
	branchkit.HandleTyped(h.plugin, "device_alias_remove", h.handleDeviceAliasRemove)
	branchkit.HandleTyped(h.plugin, "app_toggle", h.handleAppToggle)
	branchkit.HandleTyped(h.plugin, "app_alias_add", h.handleAppAliasAdd)
	branchkit.HandleTyped(h.plugin, "app_alias_remove", h.handleAppAliasRemove)
	branchkit.HandleTyped(h.plugin, "app_trait_add", h.handleAppTraitAdd)
	branchkit.HandleTyped(h.plugin, "app_trait_remove", h.handleAppTraitRemove)
	branchkit.HandleTyped(h.plugin, "set_mouse_follows_focus", h.handleSetMouseFollowsFocus)

	// Publish current audio device names as speakable collections once RPC is
	// available (OnReady), so "set output/input <device>" matches real names.
	// Re-push on hotplug so the collections track the live device set — the
	// replace is idempotent (byte-identical records are skipped platform-
	// side), so bursts and no-op
	// changes (e.g. default-device moves) don't churn the grammar.
	h.plugin.OnReady(func() { pushAudioDevicesCollections(h.plugin) })
	h.plugin.On("_platform.audio_devices.changed", func(json.RawMessage) {
		pushAudioDevicesCollections(h.plugin)
	})
	// Devices can come and go while the machine is asleep — a dock unplugged,
	// Bluetooth headphones taken out of range — and the CoreAudio property
	// listener that feeds audio_devices.changed is not running to see it. Wake
	// is the one moment the collections are guaranteed stale, so re-push.
	// Same idempotent replace as hotplug: no churn when nothing moved.
	h.plugin.On("_platform.system.did_wake", func(json.RawMessage) {
		pushAudioDevicesCollections(h.plugin)
	})

	h.plugin.Run()
}
