package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	proto "github.com/netbirdio/netbird/shared/management/proto"
)

func TestAcquirePeerLockByUID(t *testing.T) {
	s := &Server{}
	ctx := context.Background()

	unlock := s.acquirePeerLockByUID(ctx, "peer1")
	require.NotNil(t, unlock)

	// A second waiter for the same peer must block while the lock is held.
	blocked := make(chan func(), 1)
	go func() {
		blocked <- s.acquirePeerLockByUID(ctx, "peer1")
	}()
	select {
	case <-blocked:
		t.Fatal("second acquisition succeeded while the lock was held")
	case <-time.After(50 * time.Millisecond):
	}

	// Other peers are not affected by the held lock.
	other := s.acquirePeerLockByUID(ctx, "peer2")
	require.NotNil(t, other)

	unlock()
	require.NotNil(t, <-blocked)
	other()
}

func TestAcquirePeerLockByUIDCtxDone(t *testing.T) {
	s := &Server{}
	ctx := context.Background()

	unlock := s.acquirePeerLockByUID(ctx, "peer1")
	require.NotNil(t, unlock)
	defer unlock()

	ctx2, cancel := context.WithCancel(context.Background())
	got := make(chan func(), 1)
	go func() {
		got <- s.acquirePeerLockByUID(ctx2, "peer1")
	}()

	cancel()
	select {
	case u := <-got:
		require.Nil(t, u, "cancelled waiter must return a nil unlock")
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled waiter did not return")
	}

	// The lock is still held and usable afterwards.
	require.NotNil(t, s.acquirePeerLockByUID(context.Background(), "peer3"))
}

// fakeJobStream implements ManagementService_JobServer with a controllable
// Recv. Only the methods handleHandshake touches are real; the rest panic via
// the nil embedded ServerStream if a future change calls them.
type fakeJobStream struct {
	grpc.ServerStream
	ctx  context.Context
	recv func() (*proto.EncryptedMessage, error)
}

func (f *fakeJobStream) Context() context.Context               { return f.ctx }
func (f *fakeJobStream) Send(*proto.EncryptedMessage) error     { return nil }
func (f *fakeJobStream) Recv() (*proto.EncryptedMessage, error) { return f.recv() }

func TestHandleHandshakeTimeout(t *testing.T) {
	s := &Server{}
	release := make(chan struct{})
	defer close(release)
	srv := &fakeJobStream{
		ctx: context.Background(),
		recv: func() (*proto.EncryptedMessage, error) {
			<-release // never answers, like a peer that opens a stream and stalls
			return nil, errors.New("released")
		},
	}

	start := time.Now()
	_, err := s.handleHandshake(srv.Context(), srv, 50*time.Millisecond)
	require.Error(t, err)
	require.Equal(t, codes.DeadlineExceeded, status.Code(err))
	require.Less(t, time.Since(start), 5*time.Second)
}

func TestHandleHandshakeRecvError(t *testing.T) {
	s := &Server{}
	srv := &fakeJobStream{
		ctx: context.Background(),
		recv: func() (*proto.EncryptedMessage, error) {
			return nil, errors.New("transport closed")
		},
	}

	_, err := s.handleHandshake(srv.Context(), srv, time.Minute)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandleHandshakeCtxDone(t *testing.T) {
	s := &Server{}
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	defer close(release)
	srv := &fakeJobStream{
		ctx: ctx,
		recv: func() (*proto.EncryptedMessage, error) {
			<-release // stay parked so only ctx.Done() can win the select
			return nil, errors.New("released")
		},
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := s.handleHandshake(srv.Context(), srv, time.Minute)
	require.ErrorIs(t, err, context.Canceled)
}
