package server

import (
	"github.com/gorilla/mux"

	"github.com/netbirdio/netbird/management/server/eventstream"
	eventstreamhandlers "github.com/netbirdio/netbird/management/server/http/handlers/eventstream"
)

// EventStreamManager returns the event streaming integration store backed by
// eventstream.json in the datadir.
func (s *BaseServer) EventStreamManager() *eventstream.Manager {
	return Create(s, func() *eventstream.Manager {
		return eventstream.NewManager(s.Config.Datadir)
	})
}

// EventStreamForwarder returns the worker delivering queued activity events
// to each account's enabled integration.
func (s *BaseServer) EventStreamForwarder() *eventstream.Forwarder {
	return Create(s, func() *eventstream.Forwarder {
		return eventstream.NewForwarder(s.EventStreamManager())
	})
}

// registerEventStreamAPI mounts the authenticated event streaming endpoints.
func (s *BaseServer) registerEventStreamAPI(router *mux.Router) {
	eventstreamhandlers.AddEndpoints(s.PermissionsManager(), s.EventStreamManager(), router)
}
