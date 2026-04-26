package main

import (
	"sort"
	"strings"

	"github.com/branchkit/plugin-sdk-go"
)

type hidDeviceView struct {
	Name      string
	VendorID  int
	ProductID int
	Transport string
	Buttons   int
	Axes      int
	Seized    bool
}

func renderDevicesSettings(p *shared.Plugin) string {
	entries, err := p.NativeHidDevices()
	if err != nil {
		shared.Logf("system", "hid-devices error: %v", err)
		return renderTempl(Devices(nil))
	}

	var views []hidDeviceView
	for _, e := range entries {
		views = append(views, hidDeviceView{
			Name:      e.Product,
			VendorID:  e.VendorID,
			ProductID: e.ProductID,
			Transport: e.Transport,
			Buttons:   e.Buttons,
			Axes:      e.Axes,
			Seized:    e.Seized,
		})
	}

	sort.Slice(views, func(i, j int) bool {
		return strings.ToLower(views[i].Name) < strings.ToLower(views[j].Name)
	})

	return renderTempl(Devices(views))
}
