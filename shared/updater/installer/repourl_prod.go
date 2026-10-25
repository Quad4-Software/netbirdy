//go:build !devartifactsign

package installer

const (
	DefaultSigningKeysBaseURL = "https://raw.githubusercontent.com/Sudo-Ivan/netbirdy/main/publickeys/artifact-signatures"
)
