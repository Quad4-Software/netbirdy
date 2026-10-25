package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	nbconfig "github.com/netbirdio/netbird/management/internals/server/config"
)

func TestSandboxPolicyUsesConfiguredPaths(t *testing.T) {
	cfg := &CombinedConfig{}
	cfg.Server.DataDir = "/data"
	cfg.Management.DataDir = "/data"
	cfg.Server.LogFile = "/var/log/nb/server.log"
	cfg.Server.TLS.CertFile = "/certs/tls.crt"
	cfg.Server.TLS.KeyFile = "/certs/tls.key"
	cfg.Server.TLS.LetsEncrypt.DataDir = "/data/certs"
	cfg.Server.Security = &nbconfig.SecurityConfig{LogFile: "sec.jsonl", AuditLogFile: "/var/log/nb/audit.jsonl"}
	cfg.Server.UpdateMirror = &nbconfig.UpdateMirrorConfig{CacheDir: "updates"}

	p := sandboxPolicy(cfg, "/etc/netbird/config.yaml")

	assert.Contains(t, p.ReadWriteDirs, "/data")
	assert.Contains(t, p.ReadWriteDirs, "/data/certs")
	assert.Contains(t, p.ReadWriteDirs, "/data/updates")
	assert.Contains(t, p.ReadWriteFiles, "/var/log/nb/server.log")
	assert.Contains(t, p.ReadWriteFiles, "/data/sec.jsonl")
	assert.Contains(t, p.ReadWriteFiles, "/var/log/nb/audit.jsonl")
	assert.ElementsMatch(t, []string{"/etc/netbird/config.yaml", "/certs/tls.crt", "/certs/tls.key"}, p.ReadOnlyFiles)
}

func TestSandboxPolicySkipsConsoleLog(t *testing.T) {
	cfg := &CombinedConfig{}
	cfg.Server.DataDir = "/data"
	cfg.Management.DataDir = "/data"
	cfg.Server.LogFile = "console"
	p := sandboxPolicy(cfg, "")
	assert.NotContains(t, p.ReadWriteFiles, "console")
	assert.Empty(t, p.ReadOnlyFiles)
}
