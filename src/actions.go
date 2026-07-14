package main

import (
	"fmt"

	"github.com/branchkit/plugin-sdk-go"
)

// --- Per-action handlers ---

func handleVolumeUp(_ *shared.OnActionRequest) (any, error) {
	if err := volumeUp(); err != nil {
		shared.Logf("system", "volume_up: %v", err)
	}
	return nil, nil
}

func handleVolumeDown(_ *shared.OnActionRequest) (any, error) {
	if err := volumeDown(); err != nil {
		shared.Logf("system", "volume_down: %v", err)
	}
	return nil, nil
}

func handleMute(_ *shared.OnActionRequest) (any, error) {
	if err := mute(); err != nil {
		shared.Logf("system", "mute: %v", err)
	}
	return nil, nil
}

func handleUnmute(_ *shared.OnActionRequest) (any, error) {
	if err := unmute(); err != nil {
		shared.Logf("system", "unmute: %v", err)
	}
	return nil, nil
}

// Param structs (LaunchParams, SetInputParams, …) live in actions_gen.go,
// generated from plugin.json's action_types block. Edit that and re-run
// `just gen-plugins` — do not hand-declare these structs here.

func handleSetOutput(req *shared.OnActionRequest) (any, error) {
	var p SetOutputParams
	if err := req.UnmarshalParams(&p); err != nil {
		return nil, err
	}
	if p.Name == "" {
		shared.Logf("system", "set_output: no device name provided")
		return nil, nil
	}
	if err := setOutputDevice(plugin, p.Name); err != nil {
		shared.Logf("system", "set_output: %v", err)
	}
	return nil, nil
}

func handleSetInput(req *shared.OnActionRequest) (any, error) {
	var p SetInputParams
	if err := req.UnmarshalParams(&p); err != nil {
		return nil, err
	}
	if p.Name == "" {
		shared.Logf("system", "set_input: no device name provided")
		return nil, nil
	}
	if err := setInputDevice(plugin, p.Name); err != nil {
		shared.Logf("system", "set_input: %v", err)
	}
	return nil, nil
}

func handleLaunch(req *shared.OnActionRequest) (any, error) {
	var p LaunchParams
	if err := req.UnmarshalParams(&p); err != nil {
		return nil, err
	}
	if p.BundleID == "" {
		shared.Logf("system", "launch: no bundle_id provided")
		return nil, nil
	}
	newInstance := false
	if p.NewInstance != nil {
		newInstance = *p.NewInstance
	}
	if err := plugin.Call("native.launch_app", map[string]any{
		"bundle_id":    p.BundleID,
		"new_instance": newInstance,
	}, nil); err != nil {
		shared.Logf("system", "launch: %v", err)
	}
	return nil, nil
}

// handleNewWindow opens a NEW window of an app on the CURRENT Space, without
// switching to the app's existing window on another Space. `open -b` (launch)
// activates the app, which raises its frontmost window — jumping Spaces when
// that window lives elsewhere. AppleScript `make new window` instead creates a
// fresh window on the active Space. Falls back to a normal launch for apps
// with no scriptable window element.
func handleNewWindow(req *shared.OnActionRequest) (any, error) {
	var p NewWindowParams
	if err := req.UnmarshalParams(&p); err != nil {
		return nil, err
	}
	if p.BundleID == "" {
		shared.Logf("system", "new_window: no bundle_id provided")
		return nil, nil
	}
	// `application id "<bundle>"` targets by bundle id, so no app-name lookup.
	script := fmt.Sprintf(`tell application id %q to make new window`, p.BundleID)
	var res struct {
		ExitCode int    `json:"exit_code"`
		Stderr   string `json:"stderr"`
	}
	err := plugin.Call("native.run_applescript", map[string]string{"script": script}, &res)
	if err != nil || res.ExitCode != 0 {
		shared.Logf("system", "new_window: make-new-window failed for %s (err=%v exit=%d %s) — falling back to launch",
			p.BundleID, err, res.ExitCode, res.Stderr)
		plugin.Call("native.launch_app", map[string]any{
			"bundle_id":    p.BundleID,
			"new_instance": false,
		}, nil)
	}
	return nil, nil
}

func handleOpen(req *shared.OnActionRequest) (any, error) {
	var p OpenParams
	if err := req.UnmarshalParams(&p); err != nil {
		return nil, err
	}
	if p.Target == "" {
		shared.Logf("system", "open: no target provided")
		return nil, nil
	}
	if err := plugin.Call("native.open_target", map[string]any{
		"target": p.Target,
	}, nil); err != nil {
		shared.Logf("system", "open: %v", err)
	}
	return nil, nil
}
