package security

import (
	"net"
	"strings"
	"testing"
	"time"
)

func testEvent() Event {
	return Event{Timestamp: time.Unix(1700000000, 0), Type: "peer",
		Severity: SeverityHigh, AccountID: "a1", PeerID: "p1",
		SrcIP: "1.2.3.4", Rule: "peer_new_ip", Score: 75,
		ActionTaken: ActionAlert, Mode: ModeObserve, Country: "DE", ASN: 100}
}

func readN(t *testing.T, c net.PacketConn, n int) []byte {
	t.Helper()
	buf := make([]byte, 8192)
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	got := []byte{}
	for n > 0 {
		l, _, err := c.ReadFrom(buf)
		if err != nil {
			break
		}
		got = append(got, buf[:l]...)
		n--
	}
	return got
}

func TestSyslogSinkUDPFrame(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	s, err := NewSyslogSink(SyslogConfig{Network: "udp", Address: pc.LocalAddr().String()})
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Unix(1700000000, 0) }
	s.Emit(testEvent())
	got := string(readN(t, pc, 1))
	if !strings.HasPrefix(got, "<130>1 2023-11-14T22:13:20Z - netbirdy peer - - ") {
		t.Fatalf("frame: %q", got[:80])
	}
	if !strings.Contains(got, `"rule":"peer_new_ip"`) {
		t.Fatalf("payload missing json: %q", got[len(got)-120:])
	}
	s.Close()
}

func TestSyslogSinkTCPNewline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	s, err := NewSyslogSink(SyslogConfig{Network: "tcp", Address: ln.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Unix(1700000000, 0) }
	s.Emit(testEvent())
	conn, _ := ln.Accept()
	defer conn.Close()
	buf := make([]byte, 8192)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(buf[:n]), "<130>1") || !strings.HasSuffix(string(buf[:n]), "\n") {
		t.Fatalf("newline framing: %q", string(buf[:n])[:60])
	}
	s.Close()
}

func TestSyslogSinkNonBlockingDead(t *testing.T) {
	s, err := NewSyslogSink(SyslogConfig{Network: "tcp", Address: "127.0.0.1:1"}) // nothing listens
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for i := 0; i < 2000; i++ {
		s.Emit(testEvent())
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("Emit blocked on dead receiver")
	}
	s.Close()
}

func TestSyslogSinkDropOldest(t *testing.T) {
	s, err := NewSyslogSink(SyslogConfig{Network: "tcp", Address: "127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1200; i++ {
		s.Emit(testEvent())
	}
	s.mu.Lock()
	d := s.dropped
	s.mu.Unlock()
	if d == 0 {
		t.Fatal("queue overflow did not drop")
	}
	s.Close()
}
