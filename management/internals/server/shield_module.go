package server

import (
	"path/filepath"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	shieldhandlers "github.com/netbirdio/netbird/management/server/http/handlers/shield"
	nbshield "github.com/netbirdio/netbird/management/server/shield"
)

// ShieldManager returns the shield config manager singleton backed by
// shield-config.json in the datadir. An absent file means the shield config
// is unmanaged and proxies keep their local settings.
func (s *BaseServer) ShieldManager() *nbshield.Manager {
	return Create(s, func() *nbshield.Manager {
		path := filepath.Join(s.Config.Datadir, nbshield.ConfigFileName)
		mgr, err := nbshield.NewManager(path)
		if err != nil {
			log.Warnf("shield config load failed, treating as unmanaged: %v", err)
			mgr = nbshield.NewEmptyManager(path)
		}
		return mgr
	})
}

// registerShieldAPI wires the shield config source into the proxy gRPC
// server and mounts the authenticated admin and owner shield endpoints.
func (s *BaseServer) registerShieldAPI(router *mux.Router) {
	mgr := s.ShieldManager()
	grpcServer := s.ReverseProxyGRPCServer()
	grpcServer.SetShieldSource(mgr)
	shieldhandlers.AddEndpoints(s.AccountManager(), mgr, grpcServer, router)
}
