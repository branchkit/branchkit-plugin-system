package main

import (
	"encoding/json"
	"strings"
	"time"

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

func handleSetOutput(p SetOutputParams, req *shared.OnActionRequest) (any, error) {
	if p.Name == "" {
		shared.Logf("system", "set_output: no device name provided")
		return nil, nil
	}
	if err := setOutputDevice(plugin, p.Name); err != nil {
		shared.Logf("system", "set_output: %v", err)
	}
	return nil, nil
}

func handleSetInput(p SetInputParams, req *shared.OnActionRequest) (any, error) {
	if p.Name == "" {
		shared.Logf("system", "set_input: no device name provided")
		return nil, nil
	}
	if err := setInputDevice(plugin, p.Name); err != nil {
		shared.Logf("system", "set_input: %v", err)
	}
	return nil, nil
}

func handleLaunch(p LaunchParams, req *shared.OnActionRequest) (any, error) {
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
	if LoadSystemConfig().MouseFollowsFocus {
		go warpCursorToApp(p.BundleID)
	}
	return nil, nil
}

// warpCursorToApp moves the cursor to the center of the app's focused (or
// first visible) window, so scroll events land on the newly focused app.
//
// launch_app returns before macOS finishes activating: the space switch and
// window raise settle asynchronously, and windows on inactive spaces report
// placeholder bounds. Warping from pre-activation state can land on the wrong
// window or the wrong display — so wait until the app is actually frontmost,
// then query the now-visible focused window. Freshly launched apps that never
// come frontmost in time (or have no windows yet) are skipped.
func warpCursorToApp(bundleID string) {
	deadline := time.Now().Add(1500 * time.Millisecond)
	for {
		if front, err := plugin.NativeFrontmostApp(); err == nil {
			var app struct {
				BundleID string `json:"bundle_id"`
			}
			if json.Unmarshal(front.App, &app) == nil && strings.EqualFold(app.BundleID, bundleID) {
				break
			}
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	// One more beat for the window raise after the app becomes frontmost.
	time.Sleep(80 * time.Millisecond)

	wins, err := plugin.NativeAppWindows(bundleID)
	if err != nil {
		shared.Logf("system", "warp: app_windows(%s): %v", bundleID, err)
		return
	}
	var cursor *shared.NativeCursorResponse
	if cur, err := plugin.NativeCursor(); err == nil {
		cursor = cur
	}
	target := pickWarpTarget(wins, cursor)
	if target == nil {
		shared.Logf("system", "warp: %s skipped (no visible window, or cursor already inside)", bundleID)
		return
	}
	if err := plugin.NativeWarpCursor(target.X, target.Y); err != nil {
		shared.Logf("system", "warp: warp_cursor: %v", err)
		return
	}
	shared.Logf("system", "warp: %s → (%d,%d)", bundleID, target.X, target.Y)
}

type warpPoint struct {
	X, Y int
}

// pickWarpTarget returns the center of the app's focused window (falling back
// to its first non-minimized window), or nil when there is nothing to warp to
// or the cursor is already inside the target window.
func pickWarpTarget(wins []shared.WindowDetail, cursor *shared.NativeCursorResponse) *warpPoint {
	var target *shared.WindowDetail
	for i := range wins {
		w := &wins[i]
		if w.IsMinimized {
			continue
		}
		if w.IsFocused {
			target = w
			break
		}
		if target == nil {
			target = w
		}
	}
	if target == nil {
		return nil
	}
	b := target.Bounds
	if cursor != nil &&
		cursor.X >= b.X && cursor.X < b.X+b.W &&
		cursor.Y >= b.Y && cursor.Y < b.Y+b.H {
		return nil
	}
	return &warpPoint{X: b.X + b.W/2, Y: b.Y + b.H/2}
}

// handleNewWindow opens a NEW window of an app on the CURRENT Space, without
// switching to the app's existing window on another Space (which a plain
// launch would do by raising the app's frontmost window). The Space-safe
// window creation lives in the actuator (native.new_app_window); it reports
// ok=false for apps with no scriptable window element, and we fall back to a
// normal launch.
func handleNewWindow(p NewWindowParams, req *shared.OnActionRequest) (any, error) {
	if p.BundleID == "" {
		shared.Logf("system", "new_window: no bundle_id provided")
		return nil, nil
	}
	var res struct {
		OK bool `json:"ok"`
	}
	err := plugin.Call("native.new_app_window", map[string]string{"bundle_id": p.BundleID}, &res)
	if err != nil || !res.OK {
		shared.Logf("system", "new_window: %s not scriptable (err=%v) — falling back to launch",
			p.BundleID, err)
		if err := plugin.Call("native.launch_app", map[string]any{
			"bundle_id":    p.BundleID,
			"new_instance": false,
		}, nil); err != nil {
			shared.Logf("system", "new_window launch fallback: %v", err)
		}
	}
	return nil, nil
}

func handleOpen(p OpenParams, req *shared.OnActionRequest) (any, error) {
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
