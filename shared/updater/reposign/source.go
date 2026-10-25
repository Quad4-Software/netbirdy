package reposign

import (
	"fmt"
	"strings"
)

// Source describes where artifact signatures and the public key tree live.
type Source struct {
	KeysBaseURL string
	SigURLFor   func(version, file string) string
}

// GitHubSource fetches keys from the committed publickeys tree and signatures
// from release assets on GitHub.
func GitHubSource(repo string) Source {
	repo = strings.Trim(repo, "/")
	return Source{
		KeysBaseURL: "https://raw.githubusercontent.com/" + repo + "/main/publickeys/artifact-signatures/keys",
		SigURLFor: func(version, file string) string {
			return "https://github.com/" + repo + "/releases/download/" + version + "/" + file + ".sig"
		},
	}
}

// MirrorSource uses a management update mirror; keys still validate against
// the embedded root so the mirror stays untrusted.
func MirrorSource(managementURL string) Source {
	base := strings.TrimRight(managementURL, "/") + "/updates/v1"
	return Source{
		KeysBaseURL: base + "/keys",
		SigURLFor: func(version, file string) string {
			return fmt.Sprintf("%s/tag/%s/%s.sig", base, version, file)
		},
	}
}
