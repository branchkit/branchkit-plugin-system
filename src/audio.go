package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"branchkit.local/shared"
)

const volumeStep = 7

// getVolume returns the current output volume (0–100).
func getVolume() (int, error) {
	out, err := exec.Command("osascript", "-e", "output volume of (get volume settings)").Output()
	if err != nil {
		return 0, fmt.Errorf("get volume: %w", err)
	}
	vol, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("parse volume: %w", err)
	}
	return vol, nil
}

// setVolume sets the output volume (0–100).
func setVolume(vol int) error {
	if vol < 0 {
		vol = 0
	}
	if vol > 100 {
		vol = 100
	}
	cmd := fmt.Sprintf("set volume output volume %d", vol)
	return exec.Command("osascript", "-e", cmd).Run()
}

func volumeUp() error {
	vol, err := getVolume()
	if err != nil {
		return err
	}
	return setVolume(vol + volumeStep)
}

func volumeDown() error {
	vol, err := getVolume()
	if err != nil {
		return err
	}
	return setVolume(vol - volumeStep)
}

func mute() error {
	return exec.Command("osascript", "-e", "set volume output muted true").Run()
}

func unmute() error {
	return exec.Command("osascript", "-e", "set volume output muted false").Run()
}

// getMuted returns whether audio output is muted.
func getMuted() (bool, error) {
	out, err := exec.Command("osascript", "-e", "output muted of (get volume settings)").Output()
	if err != nil {
		return false, fmt.Errorf("get muted: %w", err)
	}
	return strings.TrimSpace(string(out)) == "true", nil
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
func setOutputDevice(platform *shared.PlatformClient, spokenName string) error {
	devices, err := platform.GetAudioDevices()
	if err != nil {
		return fmt.Errorf("get audio devices: %w", err)
	}
	spoken := strings.ToLower(spokenName)
	aliases := loadDeviceAliases()
	for _, d := range devices.Devices {
		if d.IsOutput && matchesDevice(d, spoken, aliases) {
			return platform.SetAudioDevice(d.UID, "output")
		}
	}
	return fmt.Errorf("no output device matching %q", spokenName)
}

// setInputDevice fuzzy-matches a spoken device name and sets the default input device.
func setInputDevice(platform *shared.PlatformClient, spokenName string) error {
	devices, err := platform.GetAudioDevices()
	if err != nil {
		return fmt.Errorf("get audio devices: %w", err)
	}
	spoken := strings.ToLower(spokenName)
	aliases := loadDeviceAliases()
	for _, d := range devices.Devices {
		if d.IsInput && matchesDevice(d, spoken, aliases) {
			return platform.SetAudioDevice(d.UID, "input")
		}
	}
	return fmt.Errorf("no input device matching %q", spokenName)
}
