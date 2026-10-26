package sandbox

// ClientPolicy limits where the client daemon can write. Reads and execution
// stay open because the daemon starts firewall, DNS and routing helpers. The
// system directories that hold programs and libraries stay read only.
func ClientPolicy() Policy {
	return Policy{
		Name:       "client daemon",
		WritesOnly: true,
		ReadWriteDirs: []string{
			"/etc", "/var", "/run", "/tmp", "/dev", "/proc", "/sys",
			"/home", "/root", "/mnt", "/media", "/srv",
		},
	}
}
