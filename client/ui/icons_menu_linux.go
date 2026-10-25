//go:build linux

package main

import _ "embed"

// 24x24: GTK4 menu rows render 22–48 px with no downscaling.

//go:embed assets/netbirdy-menu-dot-connected.png
var iconMenuDotConnected []byte

//go:embed assets/netbirdy-menu-dot-connecting.png
var iconMenuDotConnecting []byte

//go:embed assets/netbirdy-menu-dot-error.png
var iconMenuDotError []byte

//go:embed assets/netbirdy-menu-dot-idle.png
var iconMenuDotIdle []byte

//go:embed assets/netbirdy-menu-dot-offline.png
var iconMenuDotOffline []byte
