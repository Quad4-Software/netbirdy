//go:build !freebsd

package stdnet

import (
	log "github.com/sirupsen/logrus"
	"golang.zx2c4.com/wireguard/wgctrl"
)

func probeWireGuard(iFace string) bool {
	wg, err := wgctrl.New()
	if err != nil {
		log.Debugf("trying to create a wgctrl client failed with: %v", err)
		return false
	}
	defer func() {
		_ = wg.Close()
	}()

	_, err = wg.Device(iFace)
	return err == nil
}
