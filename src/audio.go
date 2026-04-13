package main

import (
	"fmt"
	"strings"

	shared "github.com/branchkit/plugin-sdk-go"
)

const volumeStep = 0.07 // ~7% per step

// getVolume returns the current output volume (0.0–1.0) and mute state via actuator RPC.
func getVolume() (float64, bool, error) {
	var resp shared.NativeVolumeResponse
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
	return plugin.Call("native.set_volume", shared.NativeSetVolumeRequest{Volume: vol}, nil)
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
	return plugin.Call("native.mute", shared.NativeMuteRequest{Muted: true}, nil)
}

func unmute() error {
	return plugin.Call("native.mute", shared.NativeMuteRequest{Muted: false}, nil)
}

// getAudioDevices fetches audio devices from the actuator via RPC.
func getAudioDevices(p *shared.Plugin) (*shared.NativeAudioDevicesResponse, error) {
	var resp shared.NativeAudioDevicesResponse
	if err := p.Call("native.audio_devices", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// setAudioDeviceViaRPC sets the default audio device via RPC.
func setAudioDeviceViaRPC(p *shared.Plugin, uid, deviceType string) error {
	return p.Call("native.set_audio_device", map[string]string{
		"uid":         uid,
		"device_type": deviceType,
	}, nil)
}

// matchesDevice checks if spokenName matches a device by name, voice hint, or alias.
func matchesDevice(d shared.AudioDevice, spoken string, aliases map[string][]string) bool {
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
func setOutputDevice(p *shared.Plugin, spokenName string) error {
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
func setInputDevice(p *shared.Plugin, spokenName string) error {
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
