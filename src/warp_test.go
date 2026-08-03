package main

import (
	"testing"

	"github.com/branchkit/plugin-sdk-go"
)

func win(x, y, w, h int, focused, minimized bool) shared.WindowDetail {
	return shared.WindowDetail{
		Bounds:      shared.WindowBounds{X: x, Y: y, W: w, H: h},
		IsFocused:   focused,
		IsMinimized: minimized,
	}
}

func TestPickWarpTargetPrefersFocusedWindow(t *testing.T) {
	wins := []shared.WindowDetail{
		win(0, 0, 100, 100, false, false),
		win(500, 500, 200, 200, true, false),
	}
	got := pickWarpTarget(wins, nil)
	if got == nil || got.X != 600 || got.Y != 600 {
		t.Fatalf("expected focused window center (600,600), got %v", got)
	}
}

func TestPickWarpTargetFallsBackToFirstVisible(t *testing.T) {
	wins := []shared.WindowDetail{
		win(0, 0, 100, 100, false, true), // minimized — skipped
		win(200, 0, 100, 100, false, false),
	}
	got := pickWarpTarget(wins, nil)
	if got == nil || got.X != 250 || got.Y != 50 {
		t.Fatalf("expected first visible window center (250,50), got %v", got)
	}
}

func TestPickWarpTargetNoWindows(t *testing.T) {
	if got := pickWarpTarget(nil, nil); got != nil {
		t.Fatalf("expected nil for no windows, got %v", got)
	}
	onlyMinimized := []shared.WindowDetail{win(0, 0, 100, 100, false, true)}
	if got := pickWarpTarget(onlyMinimized, nil); got != nil {
		t.Fatalf("expected nil for only-minimized windows, got %v", got)
	}
}

func TestPickWarpTargetSkipsWhenCursorInsideTarget(t *testing.T) {
	wins := []shared.WindowDetail{win(100, 100, 400, 300, true, false)}
	inside := &shared.NativeCursorResponse{X: 150, Y: 150}
	if got := pickWarpTarget(wins, inside); got != nil {
		t.Fatalf("expected nil when cursor already inside target, got %v", got)
	}
	outside := &shared.NativeCursorResponse{X: 50, Y: 50}
	if got := pickWarpTarget(wins, outside); got == nil || got.X != 300 || got.Y != 250 {
		t.Fatalf("expected warp to (300,250) when cursor outside, got %v", got)
	}
}
