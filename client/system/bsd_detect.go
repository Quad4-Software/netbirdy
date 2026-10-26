//go:build freebsd

package system

import (
	"encoding/json"
	"os"
	"strings"
)

// detectBSDDistro identifies the FreeBSD-based distribution and its version.
// Order: OPNsense, pfSense, os-release, then plain FreeBSD with the kernel
// release. Never returns an empty name.
func detectBSDDistro(root, kernelRelease string) (string, string) {
	if name, ver := opnsenseVersion(root); name != "" {
		return name, ver
	}
	if name, ver := pfsenseVersion(root); name != "" {
		return name, ver
	}
	if name, ver := readOsReleaseFileAt(root); name != "" {
		return name, ver
	}
	return "FreeBSD", kernelRelease
}

// opnsenseVersion reads /usr/local/opnsense/version/core, JSON with
// product_name and product_version.
func opnsenseVersion(root string) (string, string) {
	b, err := os.ReadFile(root + "usr/local/opnsense/version/core")
	if err != nil {
		return "", ""
	}
	var v struct {
		ProductName    string `json:"product_name"`
		ProductVersion string `json:"product_version"`
	}
	if json.Unmarshal(b, &v) != nil || v.ProductName == "" {
		return "", ""
	}
	name := v.ProductName
	if !strings.Contains(name, "OPNsense") {
		name = "OPNsense"
	}
	return name, v.ProductVersion
}

// pfsenseVersion reads /etc/platform for the product and /etc/version for
// the release.
func pfsenseVersion(root string) (string, string) {
	b, err := os.ReadFile(root + "etc/platform")
	if err != nil {
		return "", ""
	}
	name := strings.TrimSpace(string(b))
	if !strings.HasPrefix(name, "pfSense") {
		return "", ""
	}
	ver := ""
	if v, err := os.ReadFile(root + "etc/version"); err == nil {
		ver = strings.TrimSpace(string(v))
	}
	return name, ver
}
