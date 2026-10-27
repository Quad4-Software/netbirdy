//go:build !ios && !android

package cmd

import (
	"slices"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/shared/observability"
)

const testSentryDSN = "https://public@example.com/1"

// resetSentryDSN preserves every package-level variable applyServiceParams can
// overwrite around a test and clears the Changed marks flag manipulation left.
func resetSentryDSN(t *testing.T) {
	t.Helper()
	saved := map[*string]string{
		&sentryDSN:     sentryDSN,
		&logLevel:      logLevel,
		&daemonAddr:    daemonAddr,
		&jsonSocket:    jsonSocket,
		&managementURL: managementURL,
		&configPath:    configPath,
	}
	savedBool := map[*bool]bool{
		&enableJSONSocket:       enableJSONSocket,
		&profilesDisabled:       profilesDisabled,
		&updateSettingsDisabled: updateSettingsDisabled,
		&captureEnabled:         captureEnabled,
		&networksDisabled:       networksDisabled,
	}
	origLogFiles := logFiles
	origServiceEnvVars := serviceEnvVars
	t.Cleanup(func() {
		for p, v := range saved {
			*p = v
		}
		for p, v := range savedBool {
			*p = v
		}
		logFiles = origLogFiles
		serviceEnvVars = origServiceEnvVars
	})
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})
	serviceCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})
}

func TestSentryDSNFlagRegistered(t *testing.T) {
	assert.NotNil(t, serviceCmd.PersistentFlags().Lookup("sentry-dsn"),
		"--sentry-dsn must be a service flag so install persists it")
	assert.Equal(t, "NB_SENTRY_DSN", FlagNameToEnvVar("sentry-dsn", "NB_"))
}

func TestBuildServiceArguments_SentryDSN(t *testing.T) {
	resetSentryDSN(t)

	sentryDSN = testSentryDSN
	args := buildServiceArguments()
	idx := slices.Index(args, "--sentry-dsn")
	require.NotEqual(t, -1, idx, "a configured DSN must reach the installed service arguments")
	require.Less(t, idx+1, len(args))
	assert.Equal(t, testSentryDSN, args[idx+1])

	sentryDSN = ""
	args = buildServiceArguments()
	assert.NotContains(t, args, "--sentry-dsn")
}

func TestApplyServiceParams_SentryDSN(t *testing.T) {
	resetSentryDSN(t)

	sentryDSN = ""
	applyServiceParams(&cobra.Command{}, &serviceParams{SentryDSN: testSentryDSN})
	assert.Equal(t, testSentryDSN, sentryDSN, "a saved DSN must be restored on install")

	require.NoError(t, serviceCmd.PersistentFlags().Set("sentry-dsn", "https://explicit@example.com/2"))
	applyServiceParams(&cobra.Command{}, &serviceParams{SentryDSN: testSentryDSN})
	assert.Equal(t, "https://explicit@example.com/2", sentryDSN, "an explicit flag wins over the saved value")
}

func TestInitSentryWithoutDSN(t *testing.T) {
	flush, err := observability.InitSentry("", "netbirdy", "dev")
	require.NoError(t, err)
	require.NotNil(t, flush, "the flush func must stay safe to call when reporting is off")
	flush()
}

func TestInitSentryAcceptsDSN(t *testing.T) {
	flush, err := observability.InitSentry(testSentryDSN, "netbirdy", "dev")
	require.NoError(t, err)
	flush()
}
