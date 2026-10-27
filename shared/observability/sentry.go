// Package observability provides opt-in error tracking for the NetBirdy
// services over the Sentry protocol. Any Sentry-compatible endpoint works,
// including self-hosted GlitchTip or Bugsink. With no DSN configured every
// helper is a no-op.
package observability

import (
	"os"
	"time"

	"github.com/getsentry/sentry-go"
)

const (
	// EnvEnvironment names the environment variable that overrides the
	// Sentry environment tag reported by all services.
	EnvEnvironment = "NB_ENVIRONMENT"

	// defaultEnvironment is reported when EnvEnvironment is unset.
	defaultEnvironment = "production"

	// flushTimeout bounds how long shutdown and panic paths wait for
	// buffered events to reach the server.
	flushTimeout = 2 * time.Second
)

// InitSentry initializes the Sentry client for a service. An empty dsn
// leaves error tracking disabled and every other helper stays a no-op.
// serviceName is reported as the server name and as the service tag,
// release as the release of each event. The returned flush function drains
// buffered events before exit and is safe to defer unconditionally.
func InitSentry(dsn, serviceName, release string) (func(), error) {
	flush := func() { sentry.Flush(flushTimeout) }
	if dsn == "" {
		return flush, nil
	}

	environment := os.Getenv(EnvEnvironment)
	if environment == "" {
		environment = defaultEnvironment
	}

	if err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		ServerName:  serviceName,
		Release:     release,
		Environment: environment,
	}); err != nil {
		return flush, err
	}

	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetTag("service", serviceName)
	})

	return flush, nil
}

// CaptureError reports err to Sentry. It is a no-op for nil errors and when
// no DSN was configured.
func CaptureError(err error) {
	if err == nil {
		return
	}
	sentry.CaptureException(err)
}

// RecoverPanic reports the recovered panic value to Sentry, flushes
// buffered events, and re-panics so the process keeps its original crash
// behavior. Defer it at the top of a service entrypoint.
func RecoverPanic() {
	r := recover()
	if r == nil {
		return
	}
	sentry.CurrentHub().Recover(r)
	sentry.Flush(flushTimeout)
	panic(r)
}
