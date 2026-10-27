package observability

import (
	"testing"
)

func TestInitSentryEmptyDSN(t *testing.T) {
	flush, err := InitSentry("", "test-service", "dev")
	if err != nil {
		t.Fatalf("expected nil error for empty DSN, got %v", err)
	}
	if flush == nil {
		t.Fatal("expected non-nil flush function")
	}
	flush()
}

func TestInitSentryBadDSN(t *testing.T) {
	_, err := InitSentry("not-a-dsn", "test-service", "dev")
	if err == nil {
		t.Fatal("expected error for malformed DSN, got nil")
	}
}

func TestInitSentryValidDSN(t *testing.T) {
	flush, err := InitSentry("https://public@example.com/1", "test-service", "dev")
	if err != nil {
		t.Fatalf("expected nil error for valid DSN, got %v", err)
	}
	flush()
}

func TestCaptureErrorNil(t *testing.T) {
	CaptureError(nil)
}

func TestRecoverPanicNoPanic(t *testing.T) {
	defer RecoverPanic()
}
