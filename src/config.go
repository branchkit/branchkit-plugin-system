package main

import (
	"github.com/branchkit/plugin-sdk-go"
)

// SystemConfig rides the settings preset: plugin.json declares the fields
// with defaults, the platform materializes the composed view, this plugin
// only reads it (DESIGN_PLUGIN_SETTINGS_STORAGE.md). User gestures from the
// Apps tab relay into the user band via overrides.apply — the pattern the
// helloworld plugin's settings.go documents as the template to copy.
//
// The manifest subscribes to `_platform.collection.updated` so the mirror also
// picks up a change made ANYWHERE else (the Collections tab, another window).
// Before that subscription existed, the relay's explicit Refresh() below was
// the only thing keeping this view fresh, and an edit made elsewhere was
// invisible here until restart.
type SystemConfig struct {
	MouseFollowsFocus bool `json:"mouse_follows_focus"`
}

const configCollection = "plugin.system.config"

var configMirror *branchkit.SettingsMirror[SystemConfig]

// initConfig wires the typed mirror. Must run before plugin.Run().
func initConfig(p *branchkit.Plugin) {
	configMirror = branchkit.Settings[SystemConfig](p, configCollection)
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

// setUserConfigField relays one user gesture into the user band (plugins
// never write settings; writers: platform_only). SetUser refreshes the
// mirror in the same operation, so re-renders see the write immediately.
func setUserConfigField(key string, value any) error {
	if configMirror == nil {
		return nil
	}
	return configMirror.SetUser(key, value)
}
