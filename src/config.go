package main

import (
	"encoding/json"

	"github.com/branchkit/plugin-sdk-go"
)

// SystemConfig rides the settings preset: plugin.json declares the fields
// with defaults, the platform materializes the composed view, this plugin
// only reads it (DESIGN_PLUGIN_SETTINGS_STORAGE.md). User gestures from the
// Apps tab relay into the user band via overrides.apply.
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

// setUserConfigField relays one user gesture into the user band (plugins
// never write settings; writers: platform_only).
func setUserConfigField(key string, value any) error {
	if plugin == nil {
		return nil
	}
	raw, err := json.Marshal(map[string]any{key: value})
	if err != nil {
		return err
	}
	id := configCollection
	tenant := "_user"
	if _, err := plugin.OverridesApply(
		"patch", configCollection, nil, raw, &id, nil, &tenant,
	); err != nil {
		return err
	}
	if configMirror != nil {
		if err := configMirror.Refresh(); err != nil {
			shared.Logf("system", "config refresh after relay failed: %v", err)
		}
	}
	return nil
}
