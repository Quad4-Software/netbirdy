//go:build freebsd

package stdnet

import (
	"bytes"
	"os"
	"runtime"
	"unsafe"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/netbirdio/netbird/client/internal/wguapi"
)

type ifgreq struct {
	Name [unix.IFNAMSIZ]byte
}

// ifgroupreq mirrors the C struct passed to SIOCGIFGMEMB. The kernel union is
// 16 bytes even though only its pointer variant is used.
type ifgroupreq struct {
	Name   [unix.IFNAMSIZ]byte
	Len    uint32
	Groups *ifgreq
	_      [16 - unsafe.Sizeof(uintptr(0))]byte
}

func probeWireGuard(iFace string) bool {
	return probeKernelWireGuard(iFace) || probeUserspaceWireGuard(iFace)
}

func probeUserspaceWireGuard(iFace string) bool {
	_, err := wguapi.New().Device(iFace)
	return err == nil
}

func probeKernelWireGuard(iFace string) bool {
	if iFace == "" || len(iFace) >= unix.IFNAMSIZ {
		return false
	}

	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		log.Debugf("open FreeBSD interface ioctl socket: %v", err)
		return false
	}
	defer func() {
		_ = unix.Close(fd)
	}()

	isMember, err := ifGroupContains(fd, "wg", iFace)
	if err != nil {
		log.Debugf("query FreeBSD WireGuard interface group: %v", err)
		return false
	}
	return isMember
}

func ifGroupContains(fd int, group, iFace string) (bool, error) {
	var ifg ifgroupreq
	copy(ifg.Name[:], group)

	if err := ioctlIfgroupreq(fd, &ifg); err != nil {
		return false, err
	}

	count := int(ifg.Len / uint32(unsafe.Sizeof(ifgreq{})))
	if count == 0 {
		return false, nil
	}

	ifgrs := make([]ifgreq, count)
	ifg.Groups = &ifgrs[0]
	if err := ioctlIfgroupreq(fd, &ifg); err != nil {
		return false, err
	}
	runtime.KeepAlive(&ifg)
	runtime.KeepAlive(ifgrs)

	for i := range ifgrs {
		name := ifgrs[i].Name[:]
		if end := bytes.IndexByte(name, 0); end >= 0 {
			name = name[:end]
		}
		if string(name) == iFace {
			return true, nil
		}
	}
	return false, nil
}

func ioctlIfgroupreq(fd int, ifg *ifgroupreq) error {
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd),
		uintptr(unix.SIOCGIFGMEMB),
		uintptr(unsafe.Pointer(ifg)),
	)
	if errno != 0 {
		return os.NewSyscallError("ioctl", errno)
	}
	return nil
}
