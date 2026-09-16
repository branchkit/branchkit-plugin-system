package main

import (
	"sync"

	"github.com/branchkit/plugin-sdk-go"
)

// Host is what every handler needs: the platform handle, the settings
// mirror, the scanned app list with the mutex that guards it, and the lock
// around device-alias writes. Handlers are methods on it, so a handler's
// dependencies are visible in its signature and each mutex sits beside the
// data it protects.
type Host struct {
	plugin       *branchkit.Plugin
	configMirror *branchkit.SettingsMirror[SystemConfig]

	appsMu sync.Mutex
	apps   []AppEntry

	deviceAliasesMu sync.Mutex
}

func newHost(p *branchkit.Plugin) *Host { return &Host{plugin: p} }
