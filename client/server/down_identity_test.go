package server

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	gstatus "google.golang.org/grpc/status"

	"github.com/netbirdio/netbird/client/proto"
)

func TestDownRefusesUnidentifiedCaller(t *testing.T) {
	s := &Server{}

	_, err := s.Down(context.Background(), &proto.DownRequest{})
	if gstatus.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", gstatus.Code(err))
	}
}

func TestDownRefusesTCPCallerWithoutIdentity(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	server := grpc.NewServer()
	proto.RegisterDaemonServiceServer(server, &Server{})
	t.Cleanup(func() {
		server.Stop()
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("close listener: %v", err)
		}
	})
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve: %v", err)
		}
	}()

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close client: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = proto.NewDaemonServiceClient(conn).Down(ctx, &proto.DownRequest{})
	if gstatus.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", gstatus.Code(err))
	}
}
