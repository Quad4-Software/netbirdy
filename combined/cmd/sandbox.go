package cmd

import (
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/util/sandbox"
)

// sandboxPolicy lists the paths the combined server needs, taken from its configuration.
func sandboxPolicy(cfg *CombinedConfig, cfgPath string) sandbox.Policy {
	dataDirs := sandbox.NonEmpty(cfg.Server.DataDir, cfg.Management.DataDir, cfg.Server.TLS.LetsEncrypt.DataDir)
	inData := func(path string) string {
		if path == "" || path == "console" || path == "syslog" || filepath.IsAbs(path) || cfg.Management.DataDir == "" {
			return path
		}
		return filepath.Join(cfg.Management.DataDir, path)
	}
	p := sandbox.Policy{
		Name:          "combined server",
		ReadWriteDirs: dataDirs,
		ReadOnlyFiles: sandbox.NonEmpty(cfgPath, cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile, cfg.Server.AgentNetwork.PricingDefaultsFile),
	}
	files := sandbox.NonEmpty(
		cfg.Server.LogFile,
		inData(cfg.Server.Store.File), inData(cfg.Server.ActivityStore.File), inData(cfg.Server.AccessLogStore.File), inData(cfg.Server.AuthStore.File),
	)
	if s := cfg.Server.Security; s != nil {
		files = append(files, sandbox.NonEmpty(inData(s.LogFile), inData(s.AuditLogFile))...)
	}
	for _, f := range files {
		if f != "console" && f != "syslog" {
			p.ReadWriteFiles = append(p.ReadWriteFiles, f)
		}
	}
	if m := cfg.Server.UpdateMirror; m != nil && m.CacheDir != "" {
		p.ReadWriteDirs = append(p.ReadWriteDirs, inData(m.CacheDir))
	}
	for i, d := range p.ReadWriteDirs {
		if abs, err := filepath.Abs(d); err == nil {
			p.ReadWriteDirs[i] = abs
		}
	}
	return p
}

func applySandbox(cfg *CombinedConfig) error {
	_, err := sandbox.Apply(sandboxPolicy(cfg, configPath))
	if err != nil {
		return err
	}
	log.Debug("sandbox applied")
	return nil
}
