package device

import (
	"net"
	"net/netip"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"go.uber.org/mock/gomock"

	mocks "github.com/netbirdio/netbird/client/iface/mocks"
	"github.com/netbirdio/netbird/client/iface/wgbackend"
)

type writeTestTUN struct {
	wgbackend.TUN
	written [][]byte
}

func (d *writeTestTUN) Write(bufs [][]byte, _ int) (int, error) {
	d.written = bufs
	return len(bufs), nil
}

type noOpPacketFilter struct{}

func (noOpPacketFilter) FilterOutbound([]byte, int) bool                        { return false }
func (noOpPacketFilter) FilterInbound([]byte, int) bool                         { return false }
func (noOpPacketFilter) SetUDPPacketHook(netip.Addr, uint16, func([]byte) bool) {}
func (noOpPacketFilter) SetTCPPacketHook(netip.Addr, uint16, func([]byte) bool) {}

type byteDropFilter struct{ drop byte }

func (f byteDropFilter) FilterOutbound([]byte, int) bool { return false }
func (f byteDropFilter) FilterInbound(packet []byte, _ int) bool {
	return len(packet) > 0 && packet[0] == f.drop
}
func (byteDropFilter) SetUDPPacketHook(netip.Addr, uint16, func([]byte) bool) {}
func (byteDropFilter) SetTCPPacketHook(netip.Addr, uint16, func([]byte) bool) {}

func TestFilteredDeviceWriteNoOpFilterAvoidsAllocation(t *testing.T) {
	tun := &writeTestTUN{}
	device := newDeviceFilter(tun)
	device.SetFilter(noOpPacketFilter{})
	bufs := [][]byte{{1}, {2}, {3}}

	var written int
	var writeErr error
	allocs := testing.AllocsPerRun(100, func() {
		written, writeErr = device.Write(bufs, 0)
	})
	if writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}
	if written != len(bufs) {
		t.Fatalf("written = %d, want %d", written, len(bufs))
	}
	if allocs != 0 {
		t.Fatalf("allocations per no-op filtered write = %v, want 0", allocs)
	}
	if &tun.written[0] != &bufs[0] {
		t.Fatal("no-op filter did not pass the original packet slice through")
	}
}

func TestFilteredDeviceWriteDropsOnlyMatchingPackets(t *testing.T) {
	tun := &writeTestTUN{}
	device := newDeviceFilter(tun)
	device.SetFilter(byteDropFilter{drop: 2})
	bufs := [][]byte{{1}, {2}, {3}}

	written, err := device.Write(bufs, 0)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if written != len(bufs) {
		t.Fatalf("written = %d, want %d", written, len(bufs))
	}
	if len(tun.written) != 2 || tun.written[0][0] != 1 || tun.written[1][0] != 3 {
		t.Fatalf("underlying device received %v, want packets 1 and 3", tun.written)
	}
}

func BenchmarkFilteredDeviceWriteNoOpFilter(b *testing.B) {
	device := newDeviceFilter(&writeTestTUN{})
	device.SetFilter(noOpPacketFilter{})
	bufs := [][]byte{{1}, {2}, {3}, {4}, {5}, {6}, {7}, {8}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := device.Write(bufs, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func TestDeviceWrapperRead(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("read ICMP", func(t *testing.T) {
		ipLayer := &layers.IPv4{
			Version:  4,
			TTL:      64,
			Protocol: layers.IPProtocolICMPv4,
			SrcIP:    net.IP{192, 168, 0, 1},
			DstIP:    net.IP{100, 200, 0, 1},
		}

		icmpLayer := &layers.ICMPv4{
			TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeEchoRequest, 0),
			Id:       1,
			Seq:      1,
		}

		buffer := gopacket.NewSerializeBuffer()
		err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{},
			ipLayer,
			icmpLayer,
		)
		if err != nil {
			t.Errorf("serialize packet: %v", err)
			return
		}

		mockBufs := [][]byte{{}}
		mockSizes := []int{0}
		mockOffset := 0

		tun := mocks.NewMockDevice(ctrl)
		tun.EXPECT().Read(mockBufs, mockSizes, mockOffset).
			DoAndReturn(func(bufs [][]byte, sizes []int, offset int) (int, error) {
				bufs[0] = buffer.Bytes()
				sizes[0] = len(bufs[0])
				return 1, nil
			})

		wrapped := newDeviceFilter(tun)

		bufs := [][]byte{{}}
		sizes := []int{0}
		offset := 0

		n, err := wrapped.Read(bufs, sizes, offset)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if n != 1 {
			t.Errorf("expected n=1, got %d", n)
			return
		}
	})

	t.Run("write TCP", func(t *testing.T) {
		ipLayer := &layers.IPv4{
			Version:  4,
			TTL:      64,
			Protocol: layers.IPProtocolICMPv4,
			SrcIP:    net.IP{100, 200, 0, 9},
			DstIP:    net.IP{100, 200, 0, 10},
		}

		// create TCP layer packet
		tcpLayer := &layers.TCP{
			SrcPort: layers.TCPPort(34423),
			DstPort: layers.TCPPort(8080),
		}

		buffer := gopacket.NewSerializeBuffer()
		err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{},
			ipLayer,
			tcpLayer,
		)
		if err != nil {
			t.Errorf("serialize packet: %v", err)
			return
		}

		mockBufs := [][]byte{buffer.Bytes()}

		mockBufs[0] = buffer.Bytes()
		tun := mocks.NewMockDevice(ctrl)
		tun.EXPECT().Write(mockBufs, 0).Return(1, nil)

		wrapped := newDeviceFilter(tun)

		bufs := [][]byte{buffer.Bytes()}

		n, err := wrapped.Write(bufs, 0)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if n != 1 {
			t.Errorf("expected n=1, got %d", n)
			return
		}
	})

	t.Run("drop write UDP package", func(t *testing.T) {
		ipLayer := &layers.IPv4{
			Version:  4,
			TTL:      64,
			Protocol: layers.IPProtocolICMPv4,
			SrcIP:    net.IP{100, 200, 0, 11},
			DstIP:    net.IP{100, 200, 0, 20},
		}

		// create TCP layer packet
		tcpLayer := &layers.UDP{
			SrcPort: layers.UDPPort(27278),
			DstPort: layers.UDPPort(53),
		}

		buffer := gopacket.NewSerializeBuffer()
		err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{},
			ipLayer,
			tcpLayer,
		)
		if err != nil {
			t.Errorf("serialize packet: %v", err)
			return
		}

		mockBufs := [][]byte{}

		tun := mocks.NewMockDevice(ctrl)
		tun.EXPECT().Write(mockBufs, 0).Return(0, nil)

		filter := mocks.NewMockPacketFilter(ctrl)
		filter.EXPECT().FilterInbound(gomock.Any(), gomock.Any()).Return(true)

		wrapped := newDeviceFilter(tun)
		wrapped.filter = filter

		bufs := [][]byte{buffer.Bytes()}

		n, err := wrapped.Write(bufs, 0)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if n != 1 {
			t.Errorf("expected n=1, got %d", n)
			return
		}
	})

	t.Run("drop read UDP package", func(t *testing.T) {
		ipLayer := &layers.IPv4{
			Version:  4,
			TTL:      64,
			Protocol: layers.IPProtocolICMPv4,
			SrcIP:    net.IP{100, 200, 0, 11},
			DstIP:    net.IP{100, 200, 0, 20},
		}

		// create TCP layer packet
		tcpLayer := &layers.UDP{
			SrcPort: layers.UDPPort(19243),
			DstPort: layers.UDPPort(1024),
		}

		buffer := gopacket.NewSerializeBuffer()
		err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{},
			ipLayer,
			tcpLayer,
		)
		if err != nil {
			t.Errorf("serialize packet: %v", err)
			return
		}

		mockBufs := [][]byte{{}}
		mockSizes := []int{0}
		mockOffset := 0

		tun := mocks.NewMockDevice(ctrl)
		tun.EXPECT().Read(mockBufs, mockSizes, mockOffset).
			DoAndReturn(func(bufs [][]byte, sizes []int, offset int) (int, error) {
				bufs[0] = buffer.Bytes()
				sizes[0] = len(bufs[0])
				return 1, nil
			})
		filter := mocks.NewMockPacketFilter(ctrl)
		filter.EXPECT().FilterOutbound(gomock.Any(), gomock.Any()).Return(true)

		wrapped := newDeviceFilter(tun)
		wrapped.filter = filter

		bufs := [][]byte{{}}
		sizes := []int{0}
		offset := 0

		n, err := wrapped.Read(bufs, sizes, offset)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			return
		}
		if n != 0 {
			t.Errorf("expected n=0, got %d", n)
			return
		}
	})
}

func TestDeviceWrapperReadPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tun := mocks.NewMockDevice(ctrl)
	tun.EXPECT().Read(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(bufs [][]byte, sizes []int, offset int) (int, error) {
			// Reproduce the wintun zero-length packet panic (index out of range).
			packet := make([]byte, 0)
			return int(packet[0]), nil
		})

	wrapped := newDeviceFilter(tun)

	handlerCalled := false
	wrapped.SetPanicHandler(func() { handlerCalled = true })

	n, err := wrapped.Read([][]byte{{}}, []int{0}, 0)
	if err == nil {
		t.Errorf("expected error from recovered panic, got nil")
	}
	if n != 0 {
		t.Errorf("expected n=0, got %d", n)
	}
	if !handlerCalled {
		t.Errorf("expected panic handler to be called")
	}
}

func TestDeviceWrapperWritePanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tun := mocks.NewMockDevice(ctrl)
	tun.EXPECT().Write(gomock.Any(), gomock.Any()).
		DoAndReturn(func(bufs [][]byte, offset int) (int, error) {
			packet := make([]byte, 0)
			return int(packet[0]), nil
		})

	wrapped := newDeviceFilter(tun)

	handlerCalled := false
	wrapped.SetPanicHandler(func() { handlerCalled = true })

	n, err := wrapped.Write([][]byte{{0x45, 0x00}}, 0)
	if err == nil {
		t.Errorf("expected error from recovered panic, got nil")
	}
	if n != 0 {
		t.Errorf("expected n=0, got %d", n)
	}
	if !handlerCalled {
		t.Errorf("expected panic handler to be called")
	}
}
