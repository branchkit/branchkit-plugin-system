package main

import (
	"github.com/branchkit/plugin-sdk-go"
)

// SystemConfig rides the settings preset: plugin.json declares the fields
// with defaults, the platform materializes the composed view, this plugin
// only reads it (DESIGN_PLUGIN_SETTINGS_STORAGE.md).
//
// It no longer WRITES it either. The Apps tab declares `plugin.system.config`
// in `embeds`, so BranchKit renders the form itself above this plugin's markup
// and the edit never transits this process (notes/DESIGN_POWERBOX.md) — which
// is what makes it the user's decision on the record rather than this
// plugin's claim about one. The mirror picks the change up from
// `_platform.collection.updated`, newly subscribed in plugin.json: the relay's
// explicit Refresh() was the only thing keeping this view fresh, so a change
// made anywhere else (the Collections tab) was invisible here until restart.
type SystemConfig struct {
	MouseFollowsFocus bool `json:"mouse_follows_focus"`
}

const configCollection = "plugin.system.config"

var configMirror *shared.SettingsMirror[SystemConfig]

// initConfig wires the typed mirror. Must run before plugin.Run().
func initConfig(p *shared.Plugin) {
	configMirror = shared.Settings[SystemConfig](p, configCollection)
}

// DefaultSystemConfig mirrors the manifest defaults — the pre-Ready
// fallback only; plugin.json is the authoritative copy.
func DefaultSystemConfig() SystemConfig {
	return SystemConfig{MouseFollowsFocus: false}
}

func LoadSystemConfig() SystemConfig {
	if configMirror != nil && configMirror.Ready() {
		return configMirror.Get()
	}
	return DefaultSystemConfig()
}
