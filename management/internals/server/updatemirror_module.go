package server

import (
	"context"

	"github.com/netbirdio/netbird/management/server/store"
	"net/http"
	"time"

	"github.com/netbirdio/netbird/management/internals/modules/updatemirror"
)

// UpdateMirror returns the update mirror singleton; nil when disabled.
func (s *BaseServer) UpdateMirror() *updatemirror.Mirror {
	return Create(s, func() *updatemirror.Mirror {
		umc := s.Config.UpdateMirror
		if umc == nil || !umc.Enabled {
			return nil
		}
		poll, err := time.ParseDuration(orDefault(umc.PollInterval, "1h"))
		if err != nil {
			poll = time.Hour
		}
		m := updatemirror.New(updatemirror.Config{
			Enabled:        true,
			Repository:     umc.Repository,
			APIBaseURL:     umc.APIBaseURL,
			CacheDir:       umc.CacheDir,
			MaxCacheBytes:  umc.MaxCacheBytes,
			PollInterval:   poll,
			GitHubTokenEnv: umc.GitHubTokenEnv,
			Platforms:      umc.Platforms,
			Prefetch:       umc.Prefetch,
		}, s.updateMirrorPlatforms, s.updateMirrorTarget)
		go m.Run(context.Background())
		return m
	})
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// updateMirrorPlatforms returns "os/arch" pairs of registered peers.
func (s *BaseServer) updateMirrorPlatforms() []string {
	seen := map[string]bool{}
	var out []string
	st := s.Store()
	if st == nil {
		return nil
	}
	ctx := context.Background()
	for _, ac := range st.GetAllAccounts(ctx) {
		peers, err := st.GetAccountPeers(ctx, store.LockingStrengthUpdate, ac.Id, "", "", "")
		if err != nil {
			continue
		}
		for _, p := range peers {
			osx := p.Meta.GoOS
			arch := p.Meta.Platform
			if osx == "" {
				continue
			}
			key := osx + "/" + arch
			if !seen[key] {
				seen[key] = true
				out = append(out, key)
			}
		}
	}
	return out
}

// updateMirrorTarget resolves the union of account AutoUpdateVersion settings.
func (s *BaseServer) updateMirrorTarget() string {
	st := s.Store()
	if st == nil {
		return ""
	}
	for _, ac := range st.GetAllAccounts(context.Background()) {
		v := ac.Settings.AutoUpdateVersion
		if v != "" && v != "disabled" {
			return v
		}
	}
	return ""
}

// mountUpdateMirror registers /updates/v1/ on the router when enabled.
func mountUpdateMirror(s *BaseServer) http.Handler {
	m := s.UpdateMirror()
	if m == nil {
		return http.NotFoundHandler()
	}
	return m.Handler()
}
