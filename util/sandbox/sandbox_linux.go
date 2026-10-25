//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/landlock-lsm/go-landlock/landlock"
	llsys "github.com/landlock-lsm/go-landlock/landlock/syscall"
)

const (
	rightsRead  = llsys.AccessFSReadFile | llsys.AccessFSReadDir
	rightsExec  = llsys.AccessFSExecute
	rightsFile  = llsys.AccessFSWriteFile | llsys.AccessFSReadFile | llsys.AccessFSTruncate
	rightsWrite = llsys.AccessFSWriteFile | llsys.AccessFSRemoveDir | llsys.AccessFSRemoveFile |
		llsys.AccessFSMakeChar | llsys.AccessFSMakeDir | llsys.AccessFSMakeReg | llsys.AccessFSMakeSock |
		llsys.AccessFSMakeFifo | llsys.AccessFSMakeBlock | llsys.AccessFSMakeSym | llsys.AccessFSRefer |
		llsys.AccessFSTruncate
)

func apply(p Policy, mode Mode, restrictReads bool) (Status, error) {
	abi, err := llsys.LandlockGetABIVersion()
	if err != nil || abi < 1 {
		if mode == ModeStrict {
			return Status{Mode: mode}, fmt.Errorf("landlock is not available on this kernel")
		}
		return Status{Mode: mode}, nil
	}

	handled := landlock.AccessFSSet(rightsWrite)
	if restrictReads {
		handled |= rightsRead | rightsExec
	}
	cfg, err := landlock.NewConfig(handled)
	if err != nil {
		return Status{Mode: mode, ABI: abi}, err
	}
	c := *cfg
	if mode != ModeStrict {
		c = c.BestEffort()
	}

	access := func(rights landlock.AccessFSSet) landlock.AccessFSSet { return rights & handled }
	rwDirs := access(rightsWrite | rightsRead)
	rwFiles := access(rightsFile)

	var rules []landlock.Rule
	if restrictReads {
		rules = append(rules,
			landlock.RODirs(existingDirs(append(SystemReadOnlyDirs(), p.ReadOnlyDirs...))...).IgnoreIfMissing(),
			landlock.ROFiles(p.ReadOnlyFiles...).IgnoreIfMissing(),
		)
	}
	rules = append(rules, landlock.PathAccess(rwFiles, "/dev/null", "/dev/tty").IgnoreIfMissing())

	rwDirList := append([]string{os.TempDir(), "/tmp", "/var/tmp", "/usr/tmp"}, p.ReadWriteDirs...)
	var rwFileList []string
	for _, f := range p.ReadWriteFiles {
		if fi, err := os.Stat(f); err == nil && fi.Mode().IsRegular() {
			rwFileList = append(rwFileList, f)
		} else {
			rwDirList = append(rwDirList, filepath.Dir(f))
		}
	}
	rules = append(rules,
		landlock.PathAccess(rwDirs, rwDirList...).IgnoreIfMissing(),
		landlock.PathAccess(rwFiles, rwFileList...).IgnoreIfMissing(),
	)

	if err := c.RestrictPaths(rules...); err != nil {
		return Status{Mode: mode, ABI: abi}, err
	}
	return Status{Mode: mode, ABI: abi, Enforced: true}, nil
}

func existingDirs(in []string) []string {
	var out []string
	for _, d := range in {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			out = append(out, d)
		}
	}
	return out
}
