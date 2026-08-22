package main

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/branchkit/plugin-sdk-go"
)

// --- Device aliases persistence ---

var (
	deviceAliasesMu sync.Mutex
)

func loadDeviceAliases() map[string][]string {
	deviceAliasesMu.Lock()
	defer deviceAliasesMu.Unlock()

	if plugin == nil {
		return map[string][]string{}
	}
	rec, err := plugin.Get("plugin.system.device_aliases", "singleton")
	if err != nil {
		branchkit.Logf("system", "device aliases collection read error: %v", err)
		return map[string][]string{}
	}
	if rec != nil {
		var m map[string][]string
		if err := json.Unmarshal(rec.Payload, &m); err != nil {
			branchkit.Logf("system", "device aliases collection parse error: %v", err)
			return map[string][]string{}
		}
		return m
	}
	return map[string][]string{}
}

func saveDeviceAliases(m map[string][]string) {
	if plugin == nil {
		return
	}
	if err := plugin.Put("plugin.system.device_aliases", "singleton", m); err != nil {
		branchkit.Logf("system", "save device aliases: %v", err)
	}
}

func addDeviceAlias(uid, alias string) {
	alias = strings.TrimSpace(strings.ToLower(alias))
	if alias == "" {
		return
	}
	m := loadDeviceAliases()
	deviceAliasesMu.Lock()
	defer deviceAliasesMu.Unlock()
	for _, a := range m[uid] {
		if a == alias {
			return
		}
	}
	m[uid] = append(m[uid], alias)
	if plugin != nil {
		plugin.Put("plugin.system.device_aliases", "singleton", m)
	}
}

func removeDeviceAlias(uid, alias string) {
	alias = strings.TrimSpace(strings.ToLower(alias))
	m := loadDeviceAliases()
	deviceAliasesMu.Lock()
	defer deviceAliasesMu.Unlock()
	aliases := m[uid]
	var kept []string
	for _, a := range aliases {
		if a != alias {
			kept = append(kept, a)
		}
	}
	if len(kept) == 0 {
		delete(m, uid)
	} else {
		m[uid] = kept
	}
	if plugin != nil {
		plugin.Put("plugin.system.device_aliases", "singleton", m)
	}
}

type soundSettingsData struct {
	Volume      int
	VolumeMinus int
	VolumePlus  int
	Muted       bool
	Outputs     []deviceView
	Inputs      []deviceView
}

type deviceView struct {
	UID        string
	Name       string
	VoiceHint  string // short name for voice command (fuzzy match)
	IsDefault  bool
	DeviceType string // "output" or "input"
	Aliases    []string
}

// voiceHint returns a short speakable name for a device.
// The fuzzy matcher uses strings.Contains, so any unique substring works.
func voiceHint(name string) string {
	lower := strings.ToLower(name)
	// Strip common prefixes to get the distinctive part
	for _, prefix := range []string{"macbook air ", "macbook pro ", "built-in "} {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimPrefix(lower, prefix)
		}
	}
	return lower
}

func renderSoundSettings(p *branchkit.Plugin) string {
	vol, muted, err := getVolume()
	if err != nil {
		branchkit.Logf("system", "getVolume error: %v", err)
	}

	devList, err := getAudioDevices(p)
	if err != nil {
		branchkit.Logf("system", "GetAudioDevices error: %v", err)
		devList = &branchkit.NativeAudioDevicesResponse{}
	}

	aliases := loadDeviceAliases()

	var outputs, inputs []deviceView
	for _, d := range devList.Devices {
		hint := voiceHint(d.Name)
		devAliases := aliases[d.UID]
		if d.IsOutput {
			outputs = append(outputs, deviceView{
				UID:        d.UID,
				Name:       d.Name,
				VoiceHint:  hint,
				IsDefault:  d.IsDefaultOutput,
				DeviceType: "output",
				Aliases:    devAliases,
			})
		}
		if d.IsInput {
			inputs = append(inputs, deviceView{
				UID:        d.UID,
				Name:       d.Name,
				VoiceHint:  hint,
				IsDefault:  d.IsDefaultInput,
				DeviceType: "input",
				Aliases:    devAliases,
			})
		}
	}

	volPct := int(vol * 100)
	minusPct := volPct - int(volumeStep*100)
	if minusPct < 0 {
		minusPct = 0
	}
	plusPct := volPct + int(volumeStep*100)
	if plusPct > 100 {
		plusPct = 100
	}

	data := soundSettingsData{
		Volume:      volPct,
		VolumeMinus: minusPct,
		VolumePlus:  plusPct,
		Muted:       muted,
		Outputs:     outputs,
		Inputs:      inputs,
	}

	return renderTempl(Sound(data))
}
