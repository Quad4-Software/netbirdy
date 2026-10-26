//go:build linux

package sandbox

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	llsys "github.com/landlock-lsm/go-landlock/landlock/syscall"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const helperEnv = "SANDBOX_TEST_HELPER"

type helperArgs struct {
	Mode          Mode
	RestrictReads bool
	RW            string
	RWFile        string
	RO            string
	Outside       string
	Probe         string
}

type helperResult struct {
	Enforced     bool
	WriteRW      bool
	CreateInRW   bool
	WriteRWFile  bool
	WriteRO      bool
	ReadRO       bool
	ReadOutside  bool
	WriteOutside bool
	CreateNear   bool
	ReadSystem   bool
	ExecSystem   bool
	ExecWritable bool
	ExecBefore   bool
	ExecErr      string
	WriteTmp     bool
	DevNull      bool
}

func TestSandboxHelper(t *testing.T) {
	raw := os.Getenv(helperEnv)
	if raw == "" {
		t.Skip("helper process only")
	}
	var a helperArgs
	require.NoError(t, json.Unmarshal([]byte(raw), &a))

	script := filepath.Join(a.RW, "run.sh")
	execBefore := exec.Command(script).Run() == nil
	st, err := apply(Policy{Name: "test", ReadWriteDirs: []string{a.RW}, ReadWriteFiles: []string{a.RWFile}, ReadOnlyDirs: []string{a.RO}}, a.Mode, a.RestrictReads)
	require.NoError(t, err)

	r := helperResult{Enforced: st.Enforced}
	r.WriteRW = os.WriteFile(filepath.Join(a.RW, "existing.txt"), []byte("x"), 0o600) == nil
	r.CreateInRW = os.WriteFile(filepath.Join(a.RW, "new.txt"), []byte("x"), 0o600) == nil
	r.WriteRWFile = os.WriteFile(a.RWFile, []byte("y"), 0o600) == nil
	r.WriteRO = os.WriteFile(filepath.Join(a.RO, "ro.txt"), []byte("x"), 0o600) == nil
	_, rerr := os.ReadFile(filepath.Join(a.RO, "ro.txt"))
	r.ReadRO = rerr == nil
	_, rerr = os.ReadFile(a.Probe)
	r.ReadOutside = rerr == nil
	r.WriteOutside = os.WriteFile(filepath.Join(a.Outside, "w.txt"), []byte("x"), 0o600) == nil
	r.CreateNear = os.WriteFile(filepath.Join(filepath.Dir(a.RWFile), "sibling.txt"), []byte("x"), 0o600) == nil
	_, rerr = os.ReadFile("/etc/hostname")
	r.ReadSystem = rerr == nil || os.IsNotExist(rerr)
	r.ExecSystem = exec.Command("/bin/true").Run() == nil
	r.ExecBefore = execBefore
	execErr := exec.Command(script).Run()
	r.ExecWritable = execErr == nil
	if execErr != nil {
		r.ExecErr = execErr.Error()
	}
	r.WriteTmp = func() bool {
		f, err := os.CreateTemp("", "sbx")
		if err != nil {
			return false
		}
		_ = f.Close()
		_ = os.Remove(f.Name())
		return true
	}()
	f, err := os.OpenFile("/dev/null", os.O_WRONLY, 0)
	r.DevNull = err == nil
	if f != nil {
		_ = f.Close()
	}

	out, _ := json.Marshal(r)
	require.NoError(t, os.WriteFile(os.Getenv(helperEnv+"_OUT"), out, 0o600))
}

func runHelper(t *testing.T, mode Mode, reads bool) helperResult {
	t.Helper()
	if v, err := llsys.LandlockGetABIVersion(); err != nil || v < 1 {
		t.Skip("landlock is not available on this kernel")
	}
	base := t.TempDir()
	rw := filepath.Join(base, "rw")
	ro := filepath.Join(base, "ro")
	outside := filepath.Join(base, "outside")
	filesDir := filepath.Join(base, "files")
	tmp := filepath.Join(base, "tmp")
	for _, d := range []string{rw, ro, outside, filesDir, tmp} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(rw, "existing.txt"), []byte("a"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(rw, "run.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(ro, "ro.txt"), []byte("a"), 0o600))
	probe := filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(probe, []byte("s"), 0o600))
	rwFile := filepath.Join(filesDir, "db.sqlite")
	require.NoError(t, os.WriteFile(rwFile, []byte("a"), 0o600))

	args, _ := json.Marshal(helperArgs{Mode: mode, RestrictReads: reads, RW: rw, RWFile: rwFile, RO: ro, Outside: outside, Probe: probe})
	outFile := filepath.Join(tmp, "result.json")
	cmd := exec.Command(os.Args[0], "-test.run=^TestSandboxHelper$")
	cmd.Env = append(os.Environ(), "TMPDIR="+tmp, helperEnv+"="+string(args), helperEnv+"_OUT="+outFile)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	raw, err := os.ReadFile(outFile)
	require.NoError(t, err)
	var r helperResult
	require.NoError(t, json.Unmarshal(raw, &r))
	if r.Enforced && (r.WriteOutside || r.WriteRO) {
		t.Skip("landlock restrict succeeded but this filesystem does not enforce it")
	}
	return r
}

func TestFullPolicyRestrictsReadsWritesAndExec(t *testing.T) {
	r := runHelper(t, ModeBestEffort, true)
	assert.True(t, r.Enforced)
	assert.True(t, r.WriteRW, "existing file in a read-write dir")
	assert.True(t, r.CreateInRW, "new file in a read-write dir")
	assert.True(t, r.WriteRWFile, "single read-write file")
	assert.False(t, r.CreateNear, "a sibling of a single file must stay protected")
	assert.False(t, r.WriteRO, "read-only dir")
	assert.True(t, r.ReadRO, "read-only dir is readable")
	assert.False(t, r.ReadOutside, "files outside the policy are unreadable")
	assert.False(t, r.WriteOutside)
	assert.True(t, r.ReadSystem)
	assert.True(t, r.ExecSystem, "system binaries run")
	assert.False(t, r.ExecWritable, "a script in a writable dir cannot run")
	assert.True(t, r.WriteTmp)
	assert.True(t, r.DevNull)
}

func TestWritesOnlyPolicyKeepsReads(t *testing.T) {
	r := runHelper(t, ModeBestEffort, false)
	assert.True(t, r.Enforced)
	assert.True(t, r.WriteRW)
	assert.False(t, r.WriteRO)
	assert.False(t, r.WriteOutside)
	assert.True(t, r.ReadOutside, "reads are not restricted in this mode")
	assert.Equal(t, r.ExecBefore, r.ExecWritable, "execute is not restricted in this mode: %s", r.ExecErr)
}

func TestStrictModeEnforces(t *testing.T) {
	r := runHelper(t, ModeStrict, true)
	assert.True(t, r.Enforced)
	assert.False(t, r.ReadOutside)
}
