package peer

import "sync/atomic"

// DirectEndpointHook reports a remote peer's last known direct (non-relayed)
// ICE endpoint while the peer is connected. The engine uses it to persist
// endpoints in the offline network-map snapshot.
var DirectEndpointHook atomic.Value // stores func(peerKey, endpoint string)

// SetDirectEndpointHook installs the reporter. Pass nil to clear.
func SetDirectEndpointHook(h func(peerKey, endpoint string)) {
	DirectEndpointHook.Store(h)
}

func reportDirectEndpoint(peerKey, endpoint string) {
	if h, ok := DirectEndpointHook.Load().(func(string, string)); ok && h != nil && endpoint != "" {
		h(peerKey, endpoint)
	}
}
