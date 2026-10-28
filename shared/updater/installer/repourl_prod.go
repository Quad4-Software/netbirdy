//go:build !devartifactsign

package installer

const (
	DefaultSigningKeysBaseURL = "https://raw.githubusercontent.com/Quad4-Software/netbirdy/main/publickeys/artifact-signatures"
)
