package updater

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

// Real podman run: set NETBIRDY_UPDATER_PODMAN_TEST=1 and have the rootless
// socket available.
func TestPodmanEngine(t *testing.T) {
	if os.Getenv("NETBIRDY_UPDATER_PODMAN_TEST") != "1" {
		t.Skip("set NETBIRDY_UPDATER_PODMAN_TEST=1 to run against local podman")
	}
	sock := os.Getenv("UPDATER_SOCKET")
	if sock == "" {
		sock = "unix:///run/user/1000/podman/podman.sock"
	}
	eng, err := NewDockerEngine(sock)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// create a labeled container from a small local image
	img := "localhost/fs-e2e-bin:latest"
	name := "updtest-a"
	resp, err := eng.call(ctx, http.MethodPost, "/containers/create?name="+name,
		map[string]any{"Image": img, "Cmd": []string{"sleep", "60"}, "Labels": map[string]string{"com.docker.compose.project": "uptest", "com.docker.compose.service": "srv"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		ID string `json:"Id"`
	}
	readJSON(resp.Body, &out)
	resp.Body.Close()
	cid := out.ID
	defer eng.Remove(ctx, cid)
	defer eng.Remove(ctx, "new-"+name)
	defer eng.Remove(ctx, name+"-prev")

	ci, err := eng.FindContainer(ctx, "uptest", "srv")
	if err != nil || ci.Name != name {
		t.Fatalf("find: %v %v", ci, err)
	}
	nid, err := eng.CreateLike(ctx, ci, img, name+"-new")
	if err != nil {
		t.Fatalf("create like: %v", err)
	}
	defer eng.Remove(ctx, nid)
	if err := eng.Stop(ctx, cid); err != nil {
		t.Fatal(err)
	}
	if err := eng.Rename(ctx, cid, name+"-prev"); err != nil {
		t.Fatal(err)
	}
	if err := eng.Start(ctx, nid); err != nil {
		t.Fatal(err)
	}
	if err := eng.WaitHealthy(ctx, nid, 10*time.Second); err != nil {
		t.Fatalf("healthy: %v", err)
	}
	// confirm labels/env survived on the new container
	ci2, err := eng.inspect(ctx, nid)
	if err != nil {
		t.Fatal(err)
	}
	labelsMap, _ := ci2.Config["Labels"].(map[string]any)
	if labelsMap["com.docker.compose.project"] != "uptest" {
		t.Fatalf("labels lost: %v", ci2.Config["Labels"])
	}
	if ci2.Config["Image"] != img {
		t.Fatalf("image %v", ci2.Config["Image"])
	}
	eng.Stop(ctx, nid)
	t.Logf("full cycle ok: find/createLike/stop/rename/start/healthy")
}
