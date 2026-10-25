//go:build windows || darwin

package installer

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadSourcesOrder(t *testing.T) {
	u := New()
	u.SetManagementURL("http://mgmt.local:8081/")
	got := u.downloadSources("v1.2.3", "f.msi", "https://github.com/x/f.msi")
	if len(got) != 2 {
		t.Fatalf("sources %v", got)
	}
	if got[0] != "http://mgmt.local:8081/updates/v1/files/v1.2.3/f.msi" {
		t.Fatalf("mirror url %s", got[0])
	}
	if got[1] != "https://github.com/x/f.msi" {
		t.Fatalf("github url %s", got[1])
	}
	u2 := New()
	got2 := u2.downloadSources("v1", "f", "gh")
	if len(got2) != 1 {
		t.Fatalf("no mgmt: %v", got2)
	}
}

func TestMirrorFirstFallsThroughOn404(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("payload")) }))
	defer gh.Close()
	mirror := httptest.NewServer(http.NotFoundHandler())
	defer mirror.Close()
	u := New()
	u.SetManagementURL(mirror.URL)
	got := u.downloadSources("v9", "f", gh.URL)
	if got[0] != mirror.URL+"/updates/v1/files/v9/f" {
		t.Fatalf("%s", got[0])
	}
}
