package internal

import (
	"errors"
	"net"
	"os"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// offlineDecision is the outcome of classifying a management-connect failure.
type offlineDecision int

const (
	offlineNo     offlineDecision = iota // do not start offline
	offlineYes                           // transport failure: start offline
	offlineDelete                        // server refused: delete snapshot, do not start
)

// shouldStartOffline classifies a management login/connect error.
// Transport-level failures (unreachable server) allow offline start; a server
// that answered with an auth rejection means the snapshot must be dropped.
func shouldStartOffline(err error, snapshotErr error, now time.Time, sessionExpires *time.Time) (offlineDecision, error) {
	if err == nil {
		return offlineNo, nil
	}
	if sessionExpires != nil && !sessionExpires.After(now) {
		return offlineDelete, errors.New("cached session already expired")
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unauthenticated, codes.PermissionDenied, codes.NotFound, codes.FailedPrecondition, codes.ResourceExhausted:
			return offlineDelete, st.Err()
		case codes.Unavailable, codes.DeadlineExceeded:
			// transport failure: fall through to snapshot check
		default:
			return offlineNo, st.Err()
		}
	}
	// DNS / dial / TLS-style errors land here, or codes.Unavailable above.
	if snapshotErr != nil {
		return offlineNo, snapshotErr
	}
	return offlineYes, nil
}

// IsTransportErr reports whether err looks like a management transport
// failure rather than a server-side refusal.
func IsTransportErr(err error) bool {
	if err == nil {
		return false
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unauthenticated, codes.PermissionDenied, codes.NotFound, codes.FailedPrecondition, codes.ResourceExhausted:
			return false
		case codes.Unavailable, codes.DeadlineExceeded:
			return true
		default:
			return false
		}
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	// Non-gRPC errors (dial failures, TLS handshake, EOF before a status is
	// delivered) count as transport failures: server refusals always carry a
	// status code, which the switch above already handled.
	return true
}

// OfflineStartDelay reads NB_OFFLINE_START_DELAY (Go duration). Default 10s;
// 0 means start offline immediately after the first failed connect attempt.
func OfflineStartDelay() time.Duration {
	v := envGet("NB_OFFLINE_START_DELAY")
	if v == "" {
		return 10 * time.Second
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

func envGet(k string) string { return os.Getenv(k) }
