package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"branchkit.local/shared"
)

//go:embed templates/sound.html
var soundTemplateHTML string

// --- Device aliases persistence ---

var (
	deviceAliasesMu   sync.Mutex
	deviceAliasesPath string
)

func initDeviceAliases() {
	dir := os.Getenv("BRANCHKIT_PLUGIN_DIR")
	if dir == "" {
		dir = "."
	}
	deviceAliasesPath = filepath.Join(dir, "device_aliases.json")
}

// loadDeviceAliases reads UID → aliases map from disk.
func loadDeviceAliases() map[string][]string {
	deviceAliasesMu.Lock()
	defer deviceAliasesMu.Unlock()

	data, err := os.ReadFile(deviceAliasesPath)
	if err != nil {
		return map[string][]string{}
	}
	var m map[string][]string
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string][]string{}
	}
	return m
}

func saveDeviceAliases(m map[string][]string) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] save device aliases: %v\n", err)
		return
	}
	if err := os.WriteFile(deviceAliasesPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] write device aliases: %v\n", err)
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
	data, _ := json.MarshalIndent(m, "", "  ")
	_ = os.WriteFile(deviceAliasesPath, data, 0644)
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
	data, _ := json.MarshalIndent(m, "", "  ")
	_ = os.WriteFile(deviceAliasesPath, data, 0644)
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

func renderSoundSettings(platform *shared.PlatformClient) string {
	vol, err := getVolume()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] getVolume error: %v\n", err)
	}
	muted, err := getMuted()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] getMuted error: %v\n", err)
	}

	devList, err := platform.GetAudioDevices()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] GetAudioDevices error: %v\n", err)
		devList = &shared.AudioDeviceList{}
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

	minus := vol - volumeStep
	if minus < 0 {
		minus = 0
	}
	plus := vol + volumeStep
	if plus > 100 {
		plus = 100
	}

	data := soundSettingsData{
		Volume:      vol,
		VolumeMinus: minus,
		VolumePlus:  plus,
		Muted:       muted,
		Outputs:     outputs,
		Inputs:      inputs,
	}

	var buf bytes.Buffer
	if err := soundSettingsTemplate.Execute(&buf, data); err != nil {
		fmt.Fprintf(os.Stderr, "[SYSTEM] sound template error: %v\n", err)
		return ""
	}
	return buf.String()
}

var soundSettingsTemplate = template.Must(template.New("sound").Funcs(template.FuncMap{
	"jsEscape": func(s string) string {
		s = template.JSEscapeString(s)
		return s
	},
}).Parse(soundTemplateHTML))

