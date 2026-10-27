package idp

import (
	"fmt"

	"github.com/netbirdio/netbird/management/server/telemetry"
)

// Quad4Manager is the IdP manager for Quad4 ID, the passkey-only OIDC
// provider derived from Pocket ID. It exposes the same REST API surface
// (/api/users with X-API-KEY authentication), so it reuses the Pocket ID
// implementation while reporting its own provider name in errors.
type Quad4Manager struct {
	*PocketIdManager
}

// Quad4ClientConfig is the configuration for the Quad4 ID manager.
type Quad4ClientConfig struct {
	APIToken           string
	ManagementEndpoint string
}

// NewQuad4Manager creates a new Quad4 ID manager.
func NewQuad4Manager(config Quad4ClientConfig, appMetrics telemetry.AppMetrics) (*Quad4Manager, error) {
	manager, err := NewPocketIdManager(PocketIdClientConfig(config), appMetrics)
	if err != nil {
		return nil, fmt.Errorf("quad4 IdP configuration is incomplete: %w", err)
	}
	manager.providerName = "Quad4 ID"
	return &Quad4Manager{manager}, nil
}

var _ Manager = (*Quad4Manager)(nil)
