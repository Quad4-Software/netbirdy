//go:build (linux && !android) || freebsd

package system

import (
	"bufio"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
)

func readOsReleaseFile() (osName string, osVer string) {
	return readOsReleaseFileAt("/")
}

func readOsReleaseFileAt(root string) (osName string, osVer string) {
	var file *os.File
	for _, p := range []string{root + "etc/os-release", root + "usr/lib/os-release"} {
		f, err := os.Open(p)
		if err == nil {
			file = f
			break
		}
	}
	if file == nil {
		log.Warnf("no os-release file under %s", root)
		return "", ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		v := strings.Trim(kv[1], `\"`)
		switch kv[0] {
		case "NAME":
			osName = v
		case "VERSION_ID":
			osVer = v
		}
		if osName != "" && osVer != "" {
			break
		}
	}
	return
}
