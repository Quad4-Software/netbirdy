// Package sandbox restricts what the current process can do on the file system
// using Linux Landlock. On other platforms and on kernels without Landlock the
// restriction is skipped in best-effort mode.
package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	log "github.com/sirupsen/logrus"
)

// Environment variables that control the sandbox for every NetBirdy server
// process.
const (
	// EnvMode selects off, best-effort (default) or strict.
	EnvMode = "NB_LANDLOCK"
	// EnvReads set to false limits only writes instead of reads and writes.
	EnvReads = "NB_LANDLOCK_READS"
	// EnvExtraRO lists extra read-only paths separated by the path list separator.
	EnvExtraRO = "NB_LANDLOCK_RO"
	// EnvExtraRW lists extra read-write paths separated by the path list separator.
	EnvExtraRW = "NB_LANDLOCK_RW"
	// EnvClient selects off (default), on or strict for the client daemon.
	EnvClient = "NB_LANDLOCK_CLIENT"
)

var active atomic.Bool

// Active reports whether this process is running under an enforced policy.
func Active() bool { return active.Load() }

// Mode is the enforcement level.
type Mode string

const (
	// ModeOff applies no restriction.
	ModeOff Mode = "off"
	// ModeBestEffort applies what the running kernel supports and continues otherwise.
	ModeBestEffort Mode = "best-effort"
	// ModeStrict fails when the kernel cannot enforce the full policy.
	ModeStrict Mode = "strict"
)

// Policy describes which paths a process may use.
type Policy struct {
	// Name identifies the process in logs.
	Name string
	// ReadWriteDirs are directories with read, write, create and delete access.
	ReadWriteDirs []string
	// ReadWriteFiles are single files with read and write access. A missing file
	// grants its parent directory so that the file can be created.
	ReadWriteFiles []string
	// ReadOnlyDirs are directories with read and execute access.
	ReadOnlyDirs []string
	// ReadOnlyFiles are single files with read access.
	ReadOnlyFiles []string
	// WritesOnly limits writes and leaves reads and execution unrestricted,
	// whatever NB_LANDLOCK_READS says.
	WritesOnly bool
}

// Status reports what Apply did.
type Status struct {
	Mode     Mode
	ABI      int
	Enforced bool
}

// ModeFromEnv reads NB_LANDLOCK.
func ModeFromEnv() Mode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvMode))) {
	case "off", "false", "0", "no", "disable", "disabled":
		return ModeOff
	case "strict":
		return ModeStrict
	default:
		return ModeBestEffort
	}
}

// RestrictReadsFromEnv reports whether reads are restricted, true unless
// NB_LANDLOCK_READS is false.
func RestrictReadsFromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvReads))) {
	case "false", "0", "no", "off":
		return false
	default:
		return true
	}
}

// SystemReadOnlyDirs are the directories every process may read and execute from.
func SystemReadOnlyDirs() []string {
	dirs := []string{"/etc", "/usr", "/lib", "/lib32", "/lib64", "/libx32", "/bin", "/sbin", "/proc", "/sys", "/run", "/dev"}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	return dirs
}

// Apply enforces the policy for the whole process according to the environment.
func Apply(p Policy) (Status, error) {
	return applyMode(p, ModeFromEnv())
}

// ApplyClient enforces the policy for the client daemon. It is off unless
// NB_LANDLOCK_CLIENT is on or strict.
func ApplyClient(p Policy) (Status, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvClient))) {
	case "on", "true", "1", "yes", "best-effort":
		return applyMode(p, ModeBestEffort)
	case "strict":
		return applyMode(p, ModeStrict)
	default:
		return Status{Mode: ModeOff}, nil
	}
}

func applyMode(p Policy, mode Mode) (Status, error) {
	if mode == ModeOff {
		log.Infof("landlock: disabled for %s", p.Name)
		return Status{Mode: mode}, nil
	}
	p = withEnvPaths(p)
	st, err := apply(p, mode, RestrictReadsFromEnv() && !p.WritesOnly)
	switch {
	case err != nil && mode == ModeStrict:
		return st, err
	case err != nil:
		log.Warnf("landlock: not enforced for %s: %v", p.Name, err)
	case st.Enforced:
		active.Store(true)
		log.Infof("landlock: enforced for %s (ABI %d)", p.Name, st.ABI)
	default:
		log.Infof("landlock: not supported by this system, %s runs unrestricted", p.Name)
	}
	return st, nil
}

func withEnvPaths(p Policy) Policy {
	p.ReadOnlyDirs = append(p.ReadOnlyDirs, splitList(os.Getenv(EnvExtraRO))...)
	p.ReadWriteDirs = append(p.ReadWriteDirs, splitList(os.Getenv(EnvExtraRW))...)
	return p
}

func splitList(v string) []string {
	var out []string
	for _, s := range strings.Split(v, string(os.PathListSeparator)) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// DirOf returns the directory that holds path, or "" for an empty path.
func DirOf(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Dir(path)
}

// NonEmpty drops empty strings.
func NonEmpty(in ...string) []string {
	var out []string
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
