//go:build !android && !ios && !freebsd && !js

package services

import "context"

// Observability exposes the UI process's error-reporting configuration to the
// frontend so the browser SDK can initialise against the same DSN the Go side
// reports through.
type Observability struct {
	sentryDSN string
}

// NewObservability returns the service bound for the frontend. An empty
// sentryDSN disables error reporting on both sides.
func NewObservability(sentryDSN string) *Observability {
	return &Observability{sentryDSN: sentryDSN}
}

// SentryDSN returns the configured Sentry-compatible DSN, empty when error
// reporting is disabled. A DSN is a public client credential, not a secret.
func (s *Observability) SentryDSN(_ context.Context) string {
	return s.sentryDSN
}
