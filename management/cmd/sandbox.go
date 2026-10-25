package cmd

import (
	"path/filepath"

	nbconfig "github.com/netbirdio/netbird/management/internals/server/config"
	"github.com/netbirdio/netbird/util/sandbox"
)

func sandboxPolicy(cfg *nbconfig.Config) sandbox.Policy {
	inData := func(path string) string {
		if path == "" || path == "console" || path == "syslog" || filepath.IsAbs(path) || cfg.Datadir == "" {
			return path
		}
		return filepath.Join(cfg.Datadir, path)
	}
	p := sandbox.Policy{
		Name:          "management server",
		ReadWriteDirs: sandbox.NonEmpty(cfg.Datadir),
		ReadOnlyFiles: sandbox.NonEmpty(nbconfig.MgmtConfigPath),
	}
	if cfg.HttpConfig != nil {
		p.ReadOnlyFiles = append(p.ReadOnlyFiles, sandbox.NonEmpty(cfg.HttpConfig.CertFile, cfg.HttpConfig.CertKey)...)
	}
	files := sandbox.NonEmpty(logFile)
	if s := cfg.Security; s != nil {
		files = append(files, sandbox.NonEmpty(inData(s.LogFile), inData(s.AuditLogFile))...)
	}
	for _, f := range files {
		if f != "console" && f != "syslog" {
			p.ReadWriteFiles = append(p.ReadWriteFiles, f)
		}
	}
	if m := cfg.UpdateMirror; m != nil && m.CacheDir != "" {
		p.ReadWriteDirs = append(p.ReadWriteDirs, inData(m.CacheDir))
	}
	return p
}
