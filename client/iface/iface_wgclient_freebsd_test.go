//go:build privileged && freebsd

package iface

import "github.com/netbirdio/netbird/client/internal/wguapi"

func newTestWGClient() (testWGClient, error) {
	return wguapi.New(), nil
}
