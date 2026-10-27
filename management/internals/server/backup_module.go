package server

import (
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/management/server/backup"
	backuphandlers "github.com/netbirdio/netbird/management/server/http/handlers/backup"
)

// BackupManager returns the backup manager singleton. On store engines that
// cannot be snapshotted it still serves status and config, while every run
// fails with the snapshotter's unsupported-engine error.
func (s *BaseServer) BackupManager() *backup.Manager {
	return Create(s, func() *backup.Manager {
		snapshotter, err := backup.NewSnapshotter(s.DBConn().Engine(), s.DBConn())
		if err != nil {
			log.Warnf("store snapshots unavailable: %v", err)
			snapshotter = &backup.UnsupportedSnapshotter{Err: err}
		}
		return backup.NewManager(s.Config.Datadir, snapshotter)
	})
}

// registerBackupAPI mounts the authenticated admin and owner backup
// endpoints and starts the scheduler.
func (s *BaseServer) registerBackupAPI(router *mux.Router) {
	mgr := s.BackupManager()
	backuphandlers.AddEndpoints(s.AccountManager(), mgr, router)
}
