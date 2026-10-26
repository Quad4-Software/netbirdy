package updater

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fakeDocker(t *testing.T) (*httptest.Server, *DockerEngine) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.41/images/img@sha256:a/json", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"Id":"i1"}`)) })
	mux.HandleFunc("/v1.41/images/missing@sha256:b/json", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/v1.41/images/create", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"done"}` + "\n"))
	})
	mux.HandleFunc("/v1.41/containers/json", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Query().Get("filters"), "com.docker.compose.project") {
			t.Error("label filter missing")
		}
		w.Write([]byte(`[{"Id":"c1"}]`))
	})
	mux.HandleFunc("/v1.41/containers/c1/json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Id":"c1","Name":"/netbirdy-server","Config":{"Image":"old","Env":["A=1"],"Labels":{"l":"v"},"Healthcheck":{"Test":["curl"]}},"State":{"Status":"running","Health":{"Status":"healthy"}},"HostConfig":{"Binds":["x"]},"NetworkSettings":{"Networks":{"net":{"Aliases":["s"]}}}}`))
	})
	mux.HandleFunc("/v1.41/containers/c1/stop", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.HandleFunc("/v1.41/containers/c1", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.HandleFunc("/v1.41/containers/c1/rename", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.HandleFunc("/v1.41/containers/new-c1/start", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.HandleFunc("/v1.41/containers/new-c1/stop", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.HandleFunc("/v1.41/containers/c1/start", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	mux.HandleFunc("/v1.41/containers/new-c1/json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Config":{},"HostConfig":{},"NetworkSettings":{},"State":{"Status":"running","Health":{"Status":"healthy"}}}`))
	})
	mux.HandleFunc("/v1.41/containers/create", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), `"Image":"newimg"`) || !strings.Contains(string(b), `"l":"v"`) || !strings.Contains(string(b), `"A=1"`) {
			t.Errorf("create body %s", b)
		}
		w.WriteHeader(201)
		w.Write([]byte(`{"Id":"new-c1"}`))
	})
	srv := httptest.NewServer(mux)
	eng, _ := NewDockerEngine(srv.URL)
	return srv, eng
}

func TestDockerEngine(t *testing.T) {
	srv, eng := fakeDocker(t)
	defer srv.Close()
	ctx := context.Background()
	ok, err := eng.ImageExists(ctx, "img@sha256:a")
	if err != nil || !ok {
		t.Fatalf("exists: %v %v", ok, err)
	}
	ok, err = eng.ImageExists(ctx, "missing@sha256:b")
	if err != nil || ok {
		t.Fatalf("missing: %v %v", ok, err)
	}
	if err := eng.Pull(ctx, "img@sha256:a"); err != nil {
		t.Fatal(err)
	}
	ci, err := eng.FindContainer(ctx, "p", "s")
	if err != nil || ci.Name != "netbirdy-server" {
		t.Fatalf("find: %v %v", ci, err)
	}
	if err := eng.Stop(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	if err := eng.Rename(ctx, "c1", "c1-prev"); err != nil {
		t.Fatal(err)
	}
	nid, err := eng.CreateLike(ctx, ci, "newimg", "netbirdy-server")
	if err != nil || nid != "new-c1" {
		t.Fatalf("create: %v %v", nid, err)
	}
	if err := eng.Start(ctx, nid); err != nil {
		t.Fatal(err)
	}
	if err := eng.WaitHealthy(ctx, nid, 3*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := eng.Remove(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
}

// CreateLike keeps entrypoint/env and networks from the old container.
func TestCreateLikePreservesFields(t *testing.T) {
	srv, eng := fakeDocker(t)
	defer srv.Close()
	ctx := context.Background()
	ci, err := eng.FindContainer(ctx, "p", "s")
	if err != nil {
		t.Fatal(err)
	}
	ci.Config["Entrypoint"] = []any{"/entry", "-x"}
	ci.Config["User"] = "netbirdy"
	ci.Net = map[string]any{"Networks": map[string]any{"net": map[string]any{"Aliases": []any{"s"}}}}
	nid, err := eng.CreateLike(ctx, ci, "newimg", "n")
	if err != nil || nid != "new-c1" {
		t.Fatalf("create like: %v %v", nid, err)
	}
}

func TestWaitHealthyPaths(t *testing.T) {
	srv, eng := fakeDocker(t)
	defer srv.Close()
	ctx := context.Background()
	if err := eng.WaitHealthy(ctx, "new-c1", 3*time.Second); err != nil {
		t.Fatal(err)
	}
}
