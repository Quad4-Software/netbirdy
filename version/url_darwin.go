package version

import (
	"os/exec"
	"runtime"
)

const (
	urlMacIntel = "https://github.com/Quad4-Software/netbirdy/releases/latest"
	urlMacM1M2  = "https://github.com/Quad4-Software/netbirdy/releases/latest"
)

// DownloadUrl return with the proper download link
func DownloadUrl() string {
	cmd := exec.Command("brew", "list --formula | grep -i netbird")
	if err := cmd.Start(); err != nil {
		goto PKGINSTALL
	}

	if err := cmd.Wait(); err == nil {
		return downloadURL
	}

PKGINSTALL:
	switch runtime.GOARCH {
	case "amd64":
		return urlMacIntel
	case "arm64":
		return urlMacM1M2
	default:
		return downloadURL
	}
}
