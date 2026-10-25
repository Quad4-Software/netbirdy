package server

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func matchesPublic(p string) bool {
	for _, pat := range updateMirrorPublicPaths {
		if ok, _ := path.Match(pat, p); ok {
			return true
		}
	}
	return false
}

func TestUpdateMirrorPublicPaths(t *testing.T) {
	for _, p := range []string{
		"/updates/v1/manifest.json",
		"/updates/v1/keys/artifact-key-pub.pem",
		"/updates/v1/files/v1.2.3/netbirdy_installer_1.2.3_windows_amd64.msi",
		"/updates/v1/tag/v1.2.3/netbirdy_1.2.3_darwin_arm64.pkg.sig",
	} {
		assert.True(t, matchesPublic(p), p)
	}
	for _, p := range []string{
		"/api/updates", "/api/updates/check", "/api/updates/server", "/updates/v1/", "/updates/v1/files/a/b/c",
		"/updates/v1/other", "/api/users", "/updates/v1/keys/a/b",
	} {
		assert.False(t, matchesPublic(p), p)
	}
}
