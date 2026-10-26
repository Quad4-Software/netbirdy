package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DockerEngine talks to the Docker compatible HTTP API.
type DockerEngine struct {
	c    *http.Client
	base string
}

// NewDockerEngine accepts unix:///path or http://host:port.
func NewDockerEngine(sock string) (*DockerEngine, error) {
	tr := &http.Transport{}
	base := "http://localhost"
	if strings.HasPrefix(sock, "unix://") {
		socketPath := strings.TrimPrefix(sock, "unix://")
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		}
	} else if strings.HasPrefix(sock, "http://") {
		base = sock
	} else {
		return nil, fmt.Errorf("unsupported socket %q", sock)
	}
	return &DockerEngine{c: &http.Client{Transport: tr, Timeout: 30 * time.Second}, base: base + "/v1.41"}, nil
}

func (e *DockerEngine) call(ctx context.Context, method, path string, body any, slow bool) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, e.base+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c := e.c
	if slow {
		c = &http.Client{Transport: e.c.Transport, Timeout: 10 * time.Minute}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, string(b))
	}
	return resp, nil
}

func readJSON(r io.Reader, out any) error {
	return json.NewDecoder(io.LimitReader(r, 4<<20)).Decode(out)
}

func (e *DockerEngine) ImageExists(ctx context.Context, ref string) (bool, error) {
	resp, err := e.call(ctx, http.MethodGet, "/images/"+url.PathEscape(ref)+"/json", nil, false)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, err
	}
	resp.Body.Close()
	return true, nil
}

func (e *DockerEngine) Pull(ctx context.Context, ref string) error {
	repo, digest, _ := strings.Cut(ref, "@")
	path := "/images/create?fromImage=" + url.QueryEscape(repo)
	if digest != "" {
		path += "&tag=" + url.QueryEscape(digest)
	}
	resp, err := e.call(ctx, http.MethodPost, path, nil, true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	for dec.More() {
		var line struct {
			Error string `json:"error"`
		}
		if err := dec.Decode(&line); err != nil {
			return err
		}
		if line.Error != "" {
			return fmt.Errorf("pull: %s", line.Error)
		}
	}
	return nil
}

func (e *DockerEngine) FindContainer(ctx context.Context, project, service string) (ContainerInfo, error) {
	filters := url.QueryEscape(`{"label":["com.docker.compose.project=` + project + `","com.docker.compose.service=` + service + `"]}`)
	resp, err := e.call(ctx, http.MethodGet, "/containers/json?all=1&filters="+filters, nil, false)
	if err != nil {
		return ContainerInfo{}, err
	}
	defer resp.Body.Close()
	var list []struct {
		ID string `json:"Id"`
	}
	if err := readJSON(resp.Body, &list); err != nil {
		return ContainerInfo{}, err
	}
	if len(list) == 0 {
		return ContainerInfo{}, fmt.Errorf("container %s/%s not found", project, service)
	}
	return e.inspect(ctx, list[0].ID)
}

func (e *DockerEngine) inspect(ctx context.Context, id string) (ContainerInfo, error) {
	resp, err := e.call(ctx, http.MethodGet, "/containers/"+id+"/json", nil, false)
	if err != nil {
		return ContainerInfo{}, err
	}
	defer resp.Body.Close()
	var raw map[string]any
	if err := readJSON(resp.Body, &raw); err != nil {
		return ContainerInfo{}, err
	}
	cfg, ok := raw["Config"].(map[string]any)
	if !ok {
		return ContainerInfo{}, fmt.Errorf("inspect %s: missing Config", id)
	}
	host, ok := raw["HostConfig"].(map[string]any)
	if !ok {
		return ContainerInfo{}, fmt.Errorf("inspect %s: missing HostConfig", id)
	}
	netw, _ := raw["NetworkSettings"].(map[string]any)
	state, _ := raw["State"].(map[string]any)
	name, _ := raw["Name"].(string)
	return ContainerInfo{
		ID:     id,
		Name:   strings.TrimPrefix(name, "/"),
		Config: cfg,
		Host:   host,
		Net:    netw,
		State:  state,
	}, nil
}

func (e *DockerEngine) Stop(ctx context.Context, id string) error {
	r, err := e.call(ctx, http.MethodPost, "/containers/"+id+"/stop?t=20", nil, false)
	if err == nil {
		r.Body.Close()
	}
	return err
}

func (e *DockerEngine) Rename(ctx context.Context, id, name string) error {
	r, err := e.call(ctx, http.MethodPost, "/containers/"+id+"/rename?name="+url.QueryEscape(name), nil, false)
	if err == nil {
		r.Body.Close()
	}
	return err
}

func (e *DockerEngine) CreateLike(ctx context.Context, old ContainerInfo, newImage, name string) (string, error) {
	cfg := map[string]any{}
	for k, v := range old.Config {
		cfg[k] = v
	}
	cfg["Image"] = newImage
	// Reuse the old Config verbatim so entrypoint, env, healthcheck and the
	// rest survive; only the image is replaced. EndpointsConfig needs the
	// Networks map from NetworkSettings, not the object itself.
	networks, _ := old.Net["Networks"].(map[string]any)
	body := map[string]any{
		"Image":      newImage,
		"HostConfig": old.Host,
		"NetworkingConfig": map[string]any{
			"EndpointsConfig": networks,
		},
	}
	for k, v := range old.Config {
		if k != "Image" {
			body[k] = v
		}
	}
	resp, err := e.call(ctx, http.MethodPost, "/containers/create?name="+url.QueryEscape(name), body, false)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		ID string `json:"Id"`
	}
	if err := readJSON(resp.Body, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

func (e *DockerEngine) Start(ctx context.Context, id string) error {
	r, err := e.call(ctx, http.MethodPost, "/containers/"+id+"/start", nil, false)
	if err == nil {
		r.Body.Close()
	}
	return err
}

func (e *DockerEngine) Remove(ctx context.Context, id string) error {
	r, err := e.call(ctx, http.MethodDelete, "/containers/"+id+"?force=true", nil, false)
	if err == nil {
		r.Body.Close()
	}
	return err
}

func (e *DockerEngine) WaitHealthy(ctx context.Context, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := e.inspect(ctx, id)
		if err != nil {
			return err
		}
		st := info.State
		if st == nil {
			return fmt.Errorf("inspect %s: no State", id)
		}
		if r, _ := st["Status"].(string); r == "exited" || r == "dead" {
			return fmt.Errorf("container %s is %s", id, r)
		}
		if hs, ok := st["Health"].(map[string]any); ok {
			if s2, _ := hs["Status"].(string); s2 == "healthy" {
				return nil
			}
		} else if r, _ := st["Status"].(string); r == "running" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("unhealthy after %s", timeout)
}
