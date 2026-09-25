package main

import (
	"testing"

	branchkit "github.com/branchkit/plugin-sdk-go"
)

func TestIsAutoAggregate(t *testing.T) {
	for _, tc := range []struct {
		d    branchkit.AudioDevice
		want bool
	}{
		{branchkit.AudioDevice{Name: "CADefaultDeviceAggregate-80574-0", UID: "CADefaultDeviceAggregate-80574-0"}, true},
		{branchkit.AudioDevice{Name: "Aggregate", UID: "CADefaultDeviceAggregate-1-0"}, true},
		{branchkit.AudioDevice{Name: "MacBook Air Speakers", UID: "BuiltInSpeakerDevice"}, false},
		{branchkit.AudioDevice{Name: "My Aggregate Device", UID: "~:AMS2_Aggregate:0"}, false},
	} {
		if got := isAutoAggregate(tc.d); got != tc.want {
			t.Errorf("isAutoAggregate(%q, %q) = %v, want %v", tc.d.Name, tc.d.UID, got, tc.want)
		}
	}
}
