package main

import (
	"strings"
	"time"

	"github.com/branchkit/plugin-sdk-go"
)

// --- Per-action handlers ---

func (h *Host) handleVolumeUp(_ *branchkit.OnActionRequest) (any, error) {
	if err := h.volumeUp(); err != nil {
		branchkit.Logf("system", "volume_up: %v", err)
	}
	return nil, nil
}

func (h *Host) handleVolumeDown(_ *branchkit.OnActionRequest) (any, error) {
	if err := h.volumeDown(); err != nil {
		branchkit.Logf("system", "volume_down: %v", err)
	}
	return nil, nil
}

func (h *Host) handleMute(_ *branchkit.OnActionRequest) (any, error) {
	if err := h.mute(); err != nil {
		branchkit.Logf("system", "mute: %v", err)
	}
	return nil, nil
}

func (h *Host) handleUnmute(_ *branchkit.OnActionRequest) (any, error) {
	if err := h.unmute(); err != nil {
		branchkit.Logf("system", "unmute: %v", err)
	}
	return nil, nil
}

// Param structs (LaunchParams, SetInputParams, …) live in actions_gen.go,
// generated from plugin.json's action_types block. Edit that and re-run
// `just gen-plugins` — do not hand-declare these structs here.

func (h *Host) handleSetOutput(p SetOutputParams, req *branchkit.OnActionRequest) (any, error) {
	if p.Name == "" {
		branchkit.Logf("system", "set_output: no device name provided")
		return nil, nil
	}
	if err := h.setOutputDevice(h.plugin, p.Name); err != nil {
		branchkit.Logf("system", "set_output: %v", err)
	}
	return nil, nil
}

func (h *Host) handleSetInput(p SetInputParams, req *branchkit.OnActionRequest) (any, error) {
	if p.Name == "" {
		branchkit.Logf("system", "set_input: no device name provided")
		return nil, nil
	}
	if err := h.setInputDevice(h.plugin, p.Name); err != nil {
		branchkit.Logf("system", "set_input: %v", err)
	}
	return nil, nil
}

func (h *Host) handleLaunch(p LaunchParams, req *branchkit.OnActionRequest) (any, error) {
	if p.BundleID == "" {
		branchkit.Logf("system", "launch: no bundle_id provided")
		return nil, nil
	}
	newInstance := false
	if p.NewInstance != nil {
		newInstance = *p.NewInstance
	}
	if err := h.plugin.Call("native.launch_app", map[string]any{
		"bundle_id":    p.BundleID,
		"new_instance": newInstance,
	}, nil); err != nil {
		branchkit.Logf("system", "launch: %v", err)
	}
	if h.LoadSystemConfig().MouseFollowsFocus {
		go h.warpCursorToApp(p.BundleID)
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
func (h *Host) warpCursorToApp(bundleID string) {
	deadline := time.Now().Add(1500 * time.Millisecond)
	for {
		if front, err := h.plugin.NativeFrontmostApp(); err == nil {
			if front.App != nil && front.App.BundleID != nil && strings.EqualFold(*front.App.BundleID, bundleID) {
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

	wins, err := h.plugin.NativeAppWindows(bundleID)
	if err != nil {
		branchkit.Logf("system", "warp: app_windows(%s): %v", bundleID, err)
		return
	}
	var cursor *branchkit.NativeCursorResponse
	if cur, err := h.plugin.NativeCursor(); err == nil {
		cursor = cur
	}
	target := pickWarpTarget(wins, cursor)
	if target == nil {
		branchkit.Logf("system", "warp: %s skipped (no visible window, or cursor already inside)", bundleID)
		return
	}
	if err := h.plugin.NativeWarpCursor(target.X, target.Y); err != nil {
		branchkit.Logf("system", "warp: warp_cursor: %v", err)
		return
	}
	branchkit.Logf("system", "warp: %s → (%d,%d)", bundleID, target.X, target.Y)
}

type warpPoint struct {
	X, Y int
}

// pickWarpTarget returns the center of the app's focused window (falling back
// to its first non-minimized window), or nil when there is nothing to warp to
// or the cursor is already inside the target window.
func pickWarpTarget(wins []branchkit.WindowDetail, cursor *branchkit.NativeCursorResponse) *warpPoint {
	var target *branchkit.WindowDetail
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
func (h *Host) handleNewWindow(p NewWindowParams, req *branchkit.OnActionRequest) (any, error) {
	if p.BundleID == "" {
		branchkit.Logf("system", "new_window: no bundle_id provided")
		return nil, nil
	}
	var res struct {
		OK bool `json:"ok"`
	}
	err := h.plugin.Call("native.new_app_window", map[string]string{"bundle_id": p.BundleID}, &res)
	if err != nil || !res.OK {
		branchkit.Logf("system", "new_window: %s not scriptable (err=%v) — falling back to launch",
			p.BundleID, err)
		if err := h.plugin.Call("native.launch_app", map[string]any{
			"bundle_id":    p.BundleID,
			"new_instance": false,
		}, nil); err != nil {
			branchkit.Logf("system", "new_window launch fallback: %v", err)
		}
	}
	return nil, nil
}

func (h *Host) handleOpen(p OpenParams, req *branchkit.OnActionRequest) (any, error) {
	if p.Target == "" {
		branchkit.Logf("system", "open: no target provided")
		return nil, nil
	}
	if err := h.plugin.Call("native.open_target", map[string]any{
		"target": p.Target,
	}, nil); err != nil {
		branchkit.Logf("system", "open: %v", err)
	}
	return nil, nil
}
