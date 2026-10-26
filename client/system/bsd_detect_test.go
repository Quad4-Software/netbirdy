//go:build freebsd

package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectBSDDistro(t *testing.T) {
	mk := func(files map[string]string) string {
		d := t.TempDir()
		for p, c := range files {
			f := filepath.Join(d, p)
			os.MkdirAll(filepath.Dir(f), 0o755)
			os.WriteFile(f, []byte(c), 0o644)
		}
		return d
	}
	cases := []struct {
		name     string
		files    map[string]string
		wantName string
		wantVer  string
	}{
		{"opnsense", map[string]string{"usr/local/opnsense/version/core": `{"product_name":"OPNsense","product_version":"25.1"}`}, "OPNsense", "25.1"},
		{"opnsense bad json", map[string]string{"usr/local/opnsense/version/core": `not json`}, "FreeBSD", "14.2"},
		{"pfsense", map[string]string{"etc/platform": "pfSense\n", "etc/version": "2.7.2-RELEASE\n"}, "pfSense", "2.7.2-RELEASE"},
		{"pfsense plus", map[string]string{"etc/platform": "pfSense-Plus\n", "etc/version": "24.03\n"}, "pfSense-Plus", "24.03"},
		{"os-release", map[string]string{"etc/os-release": "NAME=\"GhostBSD\"\nVERSION_ID=\"24.0\"\n"}, "GhostBSD", "24.0"},
		{"usr lib os-release", map[string]string{"usr/lib/os-release": "NAME=HardenedBSD\nVERSION_ID=15.0\n"}, "HardenedBSD", "15.0"},
		{"fallback", map[string]string{}, "FreeBSD", "14.2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, v := detectBSDDistro(mk(tc.files)+"/", "14.2")
			if n != tc.wantName || v != tc.wantVer {
				t.Fatalf("got %q %q want %q %q", n, v, tc.wantName, tc.wantVer)
			}
		})
	}
}
