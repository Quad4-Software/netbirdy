package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModeFromEnv(t *testing.T) {
	cases := map[string]Mode{
		"": ModeBestEffort, "best-effort": ModeBestEffort, "on": ModeBestEffort, "strict": ModeStrict,
		"STRICT": ModeStrict, "off": ModeOff, "false": ModeOff, "0": ModeOff, "disabled": ModeOff, "junk": ModeBestEffort,
	}
	for in, want := range cases {
		t.Setenv(EnvMode, in)
		assert.Equal(t, want, ModeFromEnv(), in)
	}
}

func TestRestrictReadsFromEnv(t *testing.T) {
	for in, want := range map[string]bool{"": true, "true": true, "false": false, "0": false, "off": false} {
		t.Setenv(EnvReads, in)
		assert.Equal(t, want, RestrictReadsFromEnv(), in)
	}
}

func TestWithEnvPaths(t *testing.T) {
	sep := string(os.PathListSeparator)
	t.Setenv(EnvExtraRO, "/a"+sep+" /b "+sep)
	t.Setenv(EnvExtraRW, "/c")
	p := withEnvPaths(Policy{ReadOnlyDirs: []string{"/x"}})
	assert.Equal(t, []string{"/x", "/a", "/b"}, p.ReadOnlyDirs)
	assert.Equal(t, []string{"/c"}, p.ReadWriteDirs)
}

func TestApplyOffIsNoop(t *testing.T) {
	t.Setenv(EnvMode, "off")
	st, err := Apply(Policy{Name: "t"})
	assert.NoError(t, err)
	assert.False(t, st.Enforced)
}

func TestHelpers(t *testing.T) {
	assert.Equal(t, "", DirOf(""))
	assert.Equal(t, filepath.Dir("/var/log/x.log"), DirOf("/var/log/x.log"))
	assert.Equal(t, []string{"a", "b"}, NonEmpty("a", "", "b"))
	dirs := SystemReadOnlyDirs()
	assert.Contains(t, dirs, "/etc")
	assert.Contains(t, dirs, "/usr")
}

func TestApplyClientDefaultsOff(t *testing.T) {
	t.Setenv(EnvClient, "")
	st, err := ApplyClient(ClientPolicy())
	assert.NoError(t, err)
	assert.Equal(t, ModeOff, st.Mode)
	assert.False(t, st.Enforced)
	assert.False(t, Active())
}

func TestClientPolicyKeepsSystemDirsReadOnly(t *testing.T) {
	p := ClientPolicy()
	assert.True(t, p.WritesOnly)
	for _, d := range []string{"/etc", "/var", "/run", "/dev", "/tmp"} {
		assert.Contains(t, p.ReadWriteDirs, d)
	}
	for _, d := range []string{"/usr", "/bin", "/sbin", "/lib", "/boot", "/opt"} {
		assert.NotContains(t, p.ReadWriteDirs, d)
	}
}
