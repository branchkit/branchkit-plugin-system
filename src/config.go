package main

import (
	"encoding/json"

	"github.com/branchkit/plugin-sdk-go"
)

type SystemConfig struct {
	MouseFollowsFocus bool `json:"mouse_follows_focus"`
}

func DefaultSystemConfig() SystemConfig {
	return SystemConfig{MouseFollowsFocus: false}
}

func LoadSystemConfig() SystemConfig {
	conf := DefaultSystemConfig()
	if plugin == nil {
		return conf
	}
	rec, err := plugin.Get("plugin.system.config", "singleton")
	if err != nil {
		shared.Logf("system", "config collection read error: %v", err)
		return conf
	}
	if rec != nil {
		if err := json.Unmarshal(rec.Payload, &conf); err != nil {
			shared.Logf("system", "config collection parse error: %v", err)
			return DefaultSystemConfig()
		}
	}
	return conf
}

func SaveSystemConfig(conf SystemConfig) error {
	if plugin == nil {
		return nil
	}
	return plugin.Put("plugin.system.config", "singleton", conf)
}
