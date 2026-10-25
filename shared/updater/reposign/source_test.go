package reposign

import "testing"

func TestGitHubSourceURLs(t *testing.T) {
	s := GitHubSource("Sudo-Ivan/netbirdy")
	if s.KeysBaseURL != "https://raw.githubusercontent.com/Sudo-Ivan/netbirdy/main/publickeys/artifact-signatures/keys" {
		t.Fatalf("keys %s", s.KeysBaseURL)
	}
	if got := s.SigURLFor("v9.9.9", "netbirdy_x.msi"); got != "https://github.com/Sudo-Ivan/netbirdy/releases/download/v9.9.9/netbirdy_x.msi.sig" {
		t.Fatalf("sig %s", got)
	}
}

func TestMirrorSourceURLs(t *testing.T) {
	for _, base := range []string{"http://mgmt:8081/", "http://mgmt:8081", "http://mgmt:8081/prefix", "https://m.local:7300"} {
		s := MirrorSource(base)
		want := base
		for len(want) > 0 && want[len(want)-1] == '/' {
			want = want[:len(want)-1]
		}
		if s.KeysBaseURL != want+"/updates/v1/keys" {
			t.Fatalf("%s -> %s", base, s.KeysBaseURL)
		}
		if got := s.SigURLFor("v1.2.3", "f.msi"); got != want+"/updates/v1/tag/v1.2.3/f.msi.sig" {
			t.Fatalf("%s -> %s", base, got)
		}
	}
}
