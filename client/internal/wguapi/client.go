// Package wguapi implements the userspace WireGuard configuration protocol.
package wguapi

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const defaultSocketDirectory = "/var/run/wireguard"

// Client queries userspace WireGuard devices through their UAPI sockets.
type Client struct {
	socketDir string
	timeout   time.Duration
}

// New returns a client using the standard UAPI socket directory.
func New() *Client {
	return &Client{
		socketDir: defaultSocketDirectory,
		timeout:   time.Second,
	}
}

// Device returns the state of the named userspace device.
func (c *Client) Device(name string) (*wgtypes.Device, error) {
	if name == "" || strings.ContainsRune(name, '/') {
		return nil, os.ErrNotExist
	}

	conn, err := net.DialTimeout("unix", filepath.Join(c.socketDir, name+".sock"), c.timeout)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = conn.Close()
	}()

	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, fmt.Errorf("set UAPI deadline: %w", err)
	}
	if _, err := io.WriteString(conn, "get=1\n\n"); err != nil {
		return nil, fmt.Errorf("write UAPI get: %w", err)
	}

	device, err := parseDevice(conn)
	if err != nil {
		return nil, err
	}
	device.Name = name
	device.Type = wgtypes.Userspace
	return device, nil
}

// Close releases client resources. UAPI connections are opened per query, so
// there is no persistent resource to release.
func (c *Client) Close() error {
	return nil
}
