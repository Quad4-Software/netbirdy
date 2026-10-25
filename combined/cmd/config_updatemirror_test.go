package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateMirrorForwarding(t *testing.T) {
	dir := t.TempDir()
	cfg := `
server:
  listenAddress: ":8080"
  exposedAddress: "http://localhost:8080"
  dataDir: ` + dir + `
  authSecret: "test"
  update_mirror:
    enabled: true
    repository: "Sudo-Ivan/netbirdy"
    prefetch: false
management:
  dataDir: ` + dir + `
signal:
  enabled: false
relay:
  enabled: false
stun:
  enabled: false
`
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Server.UpdateMirror == nil || !c.Server.UpdateMirror.Enabled {
		t.Fatal("server.update_mirror not parsed")
	}
	mgmt, err := c.ToManagementConfig()
	if err != nil {
		t.Fatal(err)
	}
	if mgmt.UpdateMirror == nil || !mgmt.UpdateMirror.Enabled {
		t.Fatal("update_mirror did not reach management config")
	}
}
