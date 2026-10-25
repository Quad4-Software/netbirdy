package updater

import (
	"context"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var semverRe = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$`)

// Engine is the container-runtime surface the update runs against; tests use a fake.
// ContainerInfo carries the inspect fields needed to recreate a container.
type ContainerInfo struct {
	ID     string
	Name   string
	Config map[string]any // Config from inspect, Image overridden on create
	Host   map[string]any // HostConfig
	Net    map[string]any // NetworkingConfig endpoints
}

type Engine interface {
	ImageExists(ctx context.Context, ref string) (bool, error)
	Pull(ctx context.Context, ref string) error
	FindContainer(ctx context.Context, project, service string) (ContainerInfo, error)
	Stop(ctx context.Context, id string) error
	Rename(ctx context.Context, id, name string) error
	CreateLike(ctx context.Context, old ContainerInfo, newImage, name string) (string, error)
	Start(ctx context.Context, id string) error
	WaitHealthy(ctx context.Context, id string, timeout time.Duration) error
	Remove(ctx context.Context, id string) error
}

// ParseImages validates the signed images list lines image@sha256:<64 hex>.
func ParseImages(list []byte, prefix string, services []string) (map[string]string, error) {
	out := map[string]string{}
	for _, ln := range strings.Split(strings.TrimSpace(string(list)), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		at := strings.LastIndex(ln, "@sha256:")
		if at < 0 || len(ln[at+8:]) != 64 {
			return nil, fmt.Errorf("bad digest line %q", ln)
		}
		if _, err := hex.DecodeString(ln[at+8:]); err != nil {
			return nil, fmt.Errorf("bad digest hex %q", ln)
		}
		repo := ln[:at]
		if !strings.HasPrefix(repo, prefix) {
			return nil, fmt.Errorf("image %q outside prefix %q", repo, prefix)
		}
		svc := repo[len(prefix):]
		if svc == "updater" {
			continue
		}
		ok := false
		for _, s := range services {
			if svc == s {
				ok = true
			}
		}
		if !ok {
			return nil, fmt.Errorf("unknown service %q", svc)
		}
		if _, dup := out[svc]; dup {
			return nil, fmt.Errorf("duplicate service %q", svc)
		}
		out[svc] = ln
	}
	for _, s := range services {
		if _, ok := out[s]; !ok {
			return nil, fmt.Errorf("service %q missing from images list", s)
		}
	}
	return out, nil
}
