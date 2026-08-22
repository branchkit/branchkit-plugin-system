package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/branchkit/plugin-sdk-go"
)

const volumeStep = 0.07 // ~7% per step

// getVolume returns the current output volume (0.0–1.0) and mute state via actuator RPC.
func getVolume() (float64, bool, error) {
	var resp branchkit.NativeVolumeResponse
	if err := plugin.Call("native.volume", nil, &resp); err != nil {
		return 0, false, fmt.Errorf("get volume: %w", err)
	}
	return resp.Volume, resp.IsMuted, nil
}

// setVolume sets the output volume (0.0–1.0) via actuator RPC.
func setVolume(vol float64) error {
	if vol < 0 {
		vol = 0
	}
	if vol > 1 {
		vol = 1
	}
	return plugin.Call("native.set_volume", branchkit.NativeSetVolumeRequest{Volume: vol}, nil)
}

func volumeUp() error {
	vol, _, err := getVolume()
	if err != nil {
		return err
	}
	return setVolume(vol + volumeStep)
}

func volumeDown() error {
	vol, _, err := getVolume()
	if err != nil {
		return err
	}
	return setVolume(vol - volumeStep)
}

func mute() error {
	return plugin.Call("native.mute", branchkit.NativeMuteRequest{Muted: true}, nil)
}

func unmute() error {
	return plugin.Call("native.mute", branchkit.NativeMuteRequest{Muted: false}, nil)
}

// pushAudioDevicesCollections publishes current output/input device names as
// speakable named-entity collections so "set output/input <device>" recognizes
// the real device names (bounded, fully enumerated), instead of a `<text>` slot
// that fuzzy-matches against the whole command union and can't hear a device
// name whose words aren't already in it. Must run post-connect (RPC needed), so
// it's wired from OnReady, and re-wired on `_platform.audio_devices.changed`
// so hotplug keeps the collections current.
func pushAudioDevicesCollections(p *branchkit.Plugin) {
	resp, err := getAudioDevices(p)
	if err != nil {
		branchkit.Logf("system", "pushAudioDevices: %v", err)
		return
	}
	type entry struct {
		Spoken string `json:"spoken"`
	}
	var outputs, inputs []branchkit.CollectionPutEntry
	seenOut, seenIn := map[string]bool{}, map[string]bool{}
	for _, d := range resp.Devices {
		spoken := strings.ToLower(strings.TrimSpace(d.Name))
		if spoken == "" {
			continue
		}
		newOut, newIn := d.IsOutput && !seenOut[spoken], d.IsInput && !seenIn[spoken]
		if !newOut && !newIn {
			continue
		}
		raw, err := json.Marshal(entry{Spoken: spoken})
		if err != nil {
			branchkit.Logf("system", "audio devices: marshal %q: %v", spoken, err)
			return
		}
		if newOut {
			seenOut[spoken] = true
			outputs = append(outputs, branchkit.CollectionPutEntry{ID: spoken, Payload: raw})
		}
		if newIn {
			seenIn[spoken] = true
			inputs = append(inputs, branchkit.CollectionPutEntry{ID: spoken, Payload: raw})
		}
	}
	if _, err := p.Replace("audio_outputs", outputs, branchkit.ScopeCollection()); err != nil {
		branchkit.Logf("system", "replace audio_outputs: %v", err)
	}
	if _, err := p.Replace("audio_inputs", inputs, branchkit.ScopeCollection()); err != nil {
		branchkit.Logf("system", "replace audio_inputs: %v", err)
	}
}

// getAudioDevices fetches audio devices from the actuator via RPC.
func getAudioDevices(p *branchkit.Plugin) (*branchkit.NativeAudioDevicesResponse, error) {
	var resp branchkit.NativeAudioDevicesResponse
	if err := p.Call("native.audio_devices", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// setAudioDeviceViaRPC sets the default audio device via RPC.
func setAudioDeviceViaRPC(p *branchkit.Plugin, uid, deviceType string) error {
	return p.Call("native.set_audio_device", map[string]string{
		"uid":         uid,
		"device_type": deviceType,
	}, nil)
}

// matchesDevice checks if spokenName matches a device by name, voice hint, or alias.
func matchesDevice(d branchkit.AudioDevice, spoken string, aliases map[string][]string) bool {
	if strings.Contains(strings.ToLower(d.Name), spoken) {
		return true
	}
	for _, alias := range aliases[d.UID] {
		if strings.Contains(alias, spoken) || strings.Contains(spoken, alias) {
			return true
		}
	}
	return false
}

// setOutputDevice fuzzy-matches a spoken device name and sets the default output device.
func setOutputDevice(p *branchkit.Plugin, spokenName string) error {
	devices, err := getAudioDevices(p)
	if err != nil {
		return fmt.Errorf("get audio devices: %w", err)
	}
	spoken := strings.ToLower(spokenName)
	aliases := loadDeviceAliases()
	for _, d := range devices.Devices {
		if d.IsOutput && matchesDevice(d, spoken, aliases) {
			return setAudioDeviceViaRPC(p, d.UID, "output")
		}
	}
	return fmt.Errorf("no output device matching %q", spokenName)
}

// setInputDevice fuzzy-matches a spoken device name and sets the default input device.
func setInputDevice(p *branchkit.Plugin, spokenName string) error {
	devices, err := getAudioDevices(p)
	if err != nil {
		return fmt.Errorf("get audio devices: %w", err)
	}
	spoken := strings.ToLower(spokenName)
	aliases := loadDeviceAliases()
	for _, d := range devices.Devices {
		if d.IsInput && matchesDevice(d, spoken, aliases) {
			return setAudioDeviceViaRPC(p, d.UID, "input")
		}
	}
	return fmt.Errorf("no input device matching %q", spokenName)
}
