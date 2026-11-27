//go:build privileged && !freebsd

package iface

import "golang.zx2c4.com/wireguard/wgctrl"

func newTestWGClient() (testWGClient, error) {
	return wgctrl.New()
}
