package updater

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the sidecar updater settings from the environment.
type Config struct {
	Token         string
	Listen        string
	Socket        string
	Project       string
	Services      []string
	ImagePrefix   string
	ReleaseBase   string
	KeysBase      string
	DataDir       string
	BackupDir     string
	BackupsKeep   int
	HealthTimeout time.Duration
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() (Config, error) {
	tok := os.Getenv("UPDATER_TOKEN")
	if len(tok) < 24 {
		return Config{}, fmt.Errorf("UPDATER_TOKEN must be at least 24 characters")
	}
	keep, _ := strconv.Atoi(envOr("UPDATER_BACKUPS_KEEP", "5"))
	to, err := time.ParseDuration(envOr("UPDATER_HEALTH_TIMEOUT", "120s"))
	if err != nil {
		to = 120 * time.Second
	}
	return Config{
		Token:         tok,
		Listen:        envOr("UPDATER_LISTEN", ":8089"),
		Socket:        envOr("UPDATER_SOCKET", "unix:///run/podman/podman.sock"),
		Project:       envOr("UPDATER_PROJECT", "netbirdy"),
		Services:      strings.Split(envOr("UPDATER_SERVICES", "server,dashboard"), ","),
		ImagePrefix:   envOr("UPDATER_IMAGE_PREFIX", "ghcr.io/sudo-ivan/netbirdy-"),
		ReleaseBase:   envOr("UPDATER_RELEASE_BASE", "https://github.com/Sudo-Ivan/netbirdy/releases/download"),
		KeysBase:      envOr("UPDATER_KEYS_BASE", "https://raw.githubusercontent.com/Sudo-Ivan/netbirdy/main/publickeys/artifact-signatures/keys"),
		DataDir:       envOr("UPDATER_DATA_DIR", "/data"),
		BackupDir:     envOr("UPDATER_BACKUP_DIR", "/backups"),
		BackupsKeep:   keep,
		HealthTimeout: to,
	}, nil
}
