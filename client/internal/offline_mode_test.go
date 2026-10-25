package internal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestShouldStartOffline(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	expired := errors.New("expired")

	cases := []struct {
		name     string
		err      error
		snapErr  error
		expires  *time.Time
		decision offlineDecision
	}{
		{"no error", nil, nil, nil, offlineNo},
		{"unavailable+snapshot", status.Error(codes.Unavailable, "down"), nil, nil, offlineYes},
		{"deadline+snapshot", status.Error(codes.DeadlineExceeded, "slow"), nil, nil, offlineYes},
		{"unauth deletes", status.Error(codes.Unauthenticated, "bad token"), nil, nil, offlineDelete},
		{"perm denied deletes", status.Error(codes.PermissionDenied, "no"), nil, nil, offlineDelete},
		{"peer not found deletes", status.Error(codes.NotFound, "gone"), nil, nil, offlineDelete},
		{"failed precondition", status.Error(codes.FailedPrecondition, "login expired"), nil, nil, offlineDelete},
		{"unavailable no snapshot", status.Error(codes.Unavailable, "down"), errOfflineNoSnapshot, nil, offlineNo},
		{"expired snapshot", status.Error(codes.Unavailable, "down"), expired, nil, offlineNo},
		{"session passed", status.Error(codes.Unavailable, "down"), nil, &past, offlineDelete},
		{"session future ok", status.Error(codes.Unavailable, "down"), nil, &future, offlineYes},
		{"unknown code", status.Error(codes.Internal, "weird"), nil, nil, offlineNo},
		{"dial error", errors.New("dial tcp: connection refused"), nil, nil, offlineYes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := shouldStartOffline(tc.err, tc.snapErr, now, tc.expires)
			if d != tc.decision {
				t.Fatalf("decision = %d, want %d", d, tc.decision)
			}
		})
	}
}

func TestIsTransportErr(t *testing.T) {
	if IsTransportErr(status.Error(codes.Unauthenticated, "x")) {
		t.Fatal("auth err is not transport")
	}
	if !IsTransportErr(status.Error(codes.Unavailable, "x")) {
		t.Fatal("unavailable is transport")
	}
	if !IsTransportErr(errors.New("dial tcp: refused")) {
		t.Fatal("op error is transport")
	}
}

func TestManagementReconnectLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("auth refusal exits and deletes", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "networkmap.bin")
		if err := saveOfflineSnapshot(p, sampleSyncResponse(), nil, time.Now()); err != nil {
			t.Fatal(err)
		}
		c := &ConnectClient{}
		c.offlineSnapshotPath = p
		c.managementReconnectLoopWith(ctx, func(context.Context) error {
			return status.Error(codes.Unauthenticated, "bad token")
		})
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("snapshot should be deleted on auth refusal")
		}
	})

	t.Run("transport error keeps looping", func(t *testing.T) {
		var calls atomic.Int32
		inner, innerCancel := context.WithCancel(ctx)
		c := &ConnectClient{}
		done := make(chan struct{})
		go func() {
			c.managementReconnectLoopWith(inner, func(context.Context) error {
				calls.Add(1)
				if calls.Load() >= 3 {
					innerCancel()
				}
				return status.Error(codes.Unavailable, "down")
			})
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(12 * time.Second):
			t.Fatal("loop did not exit")
		}
		if calls.Load() < 3 {
			t.Fatalf("expected retries, got %d", calls.Load())
		}
	})

	t.Run("success exits", func(t *testing.T) {
		c := &ConnectClient{}
		done := make(chan struct{})
		go func() {
			c.managementReconnectLoopWith(ctx, func(context.Context) error { return nil })
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("loop did not exit on success")
		}
	})

	t.Run("ctx cancel exits", func(t *testing.T) {
		inner, innerCancel := context.WithCancel(ctx)
		innerCancel()
		c := &ConnectClient{}
		done := make(chan struct{})
		go func() {
			c.managementReconnectLoopWith(inner, func(context.Context) error {
				return status.Error(codes.Unavailable, "down")
			})
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("loop did not exit on cancel")
		}
	})
}
