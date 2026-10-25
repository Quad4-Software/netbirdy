package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/netbirdio/netbird/management/server/security"

	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/metric"

	serverauth "github.com/netbirdio/netbird/management/server/auth"
	nbcontext "github.com/netbirdio/netbird/management/server/context"
	"github.com/netbirdio/netbird/management/server/http/middleware/bypass"
	"github.com/netbirdio/netbird/management/server/types"
	"github.com/netbirdio/netbird/shared/auth"
	"github.com/netbirdio/netbird/shared/management/http/util"
	"github.com/netbirdio/netbird/shared/management/status"
	"github.com/netbirdio/netbird/shared/ratelimit"
)

type EnsureAccountFunc func(ctx context.Context, userAuth auth.UserAuth) (string, string, error)
type SyncUserJWTGroupsFunc func(ctx context.Context, userAuth auth.UserAuth) error

type GetUserFromUserAuthFunc func(ctx context.Context, userAuth auth.UserAuth) (*types.User, error)

// AuthMiddleware middleware to verify personal access tokens (PAT) and JWT tokens
type AuthMiddleware struct {
	authManager         serverauth.Manager
	ensureAccount       EnsureAccountFunc
	getUserFromUserAuth GetUserFromUserAuthFunc
	syncUserJWTGroups   SyncUserJWTGroupsFunc
	rateLimiter         *ratelimit.APIRateLimiter
	patUsageTracker     *PATUsageTracker
	security            *security.Engine
	userLoginDedup      *dedupCache
}

// NewAuthMiddleware instance constructor
func NewAuthMiddleware(
	authManager serverauth.Manager,
	ensureAccount EnsureAccountFunc,
	syncUserJWTGroups SyncUserJWTGroupsFunc,
	getUserFromUserAuth GetUserFromUserAuthFunc,
	rateLimiter *ratelimit.APIRateLimiter,
	meter metric.Meter,
) *AuthMiddleware {
	var patUsageTracker *PATUsageTracker
	if meter != nil {
		var err error
		patUsageTracker, err = NewPATUsageTracker(context.Background(), meter)
		if err != nil {
			log.Errorf("Failed to create PAT usage tracker: %s", err)
		}
	}

	return &AuthMiddleware{
		authManager:         authManager,
		ensureAccount:       ensureAccount,
		syncUserJWTGroups:   syncUserJWTGroups,
		getUserFromUserAuth: getUserFromUserAuth,
		rateLimiter:         rateLimiter,
		patUsageTracker:     patUsageTracker,
		userLoginDedup:      newDedupCache(),
	}
}

var packageSecurity *security.Engine

// SetSecurityEngine attaches the anomaly engine for middleware built
// through BuildMiddleware, which does not return the instance.
func SetSecurityEngine(e *security.Engine) { packageSecurity = e }

// SetSecurityEngine attaches the anomaly engine; nil disables the hooks.
func (m *AuthMiddleware) SetSecurityEngine(e *security.Engine) {
	m.security = e
	if e != nil && m.userLoginDedup == nil {
		m.userLoginDedup = newDedupCache()
	}
}

// dedupCache bounds first-observation-per-hour for user logins.
type dedupCache struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newDedupCache() *dedupCache {
	return &dedupCache{seen: make(map[string]time.Time)}
}

// firstInHour returns true once per key per hour.
func (d *dedupCache) firstInHour(key string, now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.seen[key]; ok && now.Sub(t) < time.Hour {
		return false
	}
	if len(d.seen) > 100000 {
		for k := range d.seen {
			delete(d.seen, k) // crude eviction when at the bound
			break
		}
	}
	d.seen[key] = now
	return true
}

// requestIP parses the remote address (no XFF trust here: only the trusted
// proxy path sets it, which this middleware does not consume).
func requestIP(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip, err := netip.ParseAddr(host)
	return ip, err == nil
}

// tokenID returns a stable non-reversible identifier for a PAT.
func securityTokenID(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:6])
}

// SetSecurityEngine kept above; hooks below.

// engine resolves the attached engine (instance field wins, else package).
func (m *AuthMiddleware) engine() *security.Engine {
	if m.security != nil {
		return m.security
	}
	return packageSecurity
}

func _unused() {}

// Handler method of the middleware which authenticates a user either by JWT claims or by PAT
func (m *AuthMiddleware) Handler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bypass.ShouldBypass(r.URL.Path, h, w, r) {
			return
		}

		authHeader := strings.Split(r.Header.Get("Authorization"), " ")
		authType := strings.ToLower(authHeader[0])

		// fallback to token when receive pat as bearer
		if len(authHeader) >= 2 && authType == "bearer" && strings.HasPrefix(authHeader[1], "nbp_") {
			authType = "token"
			authHeader[0] = authType
		}

		switch authType {
		case "bearer":
			if err := m.checkJWTFromRequest(r, authHeader); err != nil {
				log.WithContext(r.Context()).Errorf("Error when validating JWT: %s", err.Error())
				if sec := m.engine(); sec != nil {
					if src, ok := requestIP(r); ok {
						sec.ObserveAuthFailure(r.Context(), security.AuthFailure{
							Kind: security.AuthKindAPI, SourceIP: src, Subject: securityTokenID(authHeader[1]),
						})
					}
				}
				util.WriteError(r.Context(), status.Errorf(status.Unauthorized, "token invalid"), w)
				return
			}
			if m.security != nil {
				if src, ok := requestIP(r); ok && m.userLoginDedup.firstInHour(securityTokenID(authHeader[1]), time.Now()) {
					m.security.ObserveUserLogin(r.Context(), "", securityTokenID(authHeader[1]), src)
				}
			}
			h.ServeHTTP(w, r)
		case "token":
			if err := m.checkPATFromRequest(r, authHeader); err != nil {
				log.WithContext(r.Context()).Debugf("Error when validating PAT: %s", err.Error())
				if sec := m.engine(); sec != nil {
					if src, ok := requestIP(r); ok {
						sec.ObserveAuthFailure(r.Context(), security.AuthFailure{
							Kind: security.AuthKindAPI, SourceIP: src, Subject: securityTokenID(authHeader[1]),
						})
					}
				}
				// Check if it's a status error, otherwise default to Unauthorized
				if _, ok := status.FromError(err); !ok {
					err = status.Errorf(status.Unauthorized, "token invalid")
				}
				util.WriteError(r.Context(), err, w)
				return
			}
			if m.security != nil {
				if src, ok := requestIP(r); ok {
					m.security.ObservePATUse(r.Context(), "", securityTokenID(authHeader[1]), src)
				}
			}
			h.ServeHTTP(w, r)
		default:
			util.WriteError(r.Context(), status.Errorf(status.Unauthorized, "no valid authentication provided"), w)
			return
		}
	})
}

// CheckJWTFromRequest checks if the JWT is valid
func (m *AuthMiddleware) checkJWTFromRequest(r *http.Request, authHeaderParts []string) error {
	token, err := getTokenFromJWTRequest(authHeaderParts)

	// If an error occurs, call the error handler and return an error
	if err != nil {
		return fmt.Errorf("error extracting token: %w", err)
	}

	ctx := r.Context()

	userAuth, validatedToken, err := m.authManager.ValidateAndParseToken(ctx, token)
	if err != nil {
		return err
	}

	// Email is now extracted in ToUserAuth (from claims or userinfo endpoint)
	// Available as userAuth.Email

	// we need to call this method because if user is new, we will automatically add it to existing or create a new account
	accountId, _, err := m.ensureAccount(ctx, userAuth)
	if err != nil {
		return err
	}

	if userAuth.AccountId != accountId {
		log.WithContext(ctx).Tracef("Auth middleware sets accountId from ensure, before %s, now %s", userAuth.AccountId, accountId)
		userAuth.AccountId = accountId
	}

	userAuth, err = m.authManager.EnsureUserAccessByJWTGroups(ctx, userAuth, validatedToken)
	if err != nil {
		return err
	}

	// Detach the group-sync write from the request's cancellation: the dashboard
	// SPA aborts in-flight requests on re-render, which would otherwise cancel the
	// DB transaction mid-write and silently drop the synced groups. Context values
	// (request id, logger) are preserved; the store bounds the tx with its own timeout.
	err = m.syncUserJWTGroups(context.WithoutCancel(ctx), userAuth)
	if err != nil {
		log.WithContext(ctx).Errorf("HTTP server failed to sync user JWT groups: %s", err)
	}

	_, err = m.getUserFromUserAuth(ctx, userAuth)
	if err != nil {
		log.WithContext(ctx).Errorf("HTTP server failed to update user from user auth: %s", err)
		return err
	}

	// propagates ctx change to upstream middleware
	*r = *nbcontext.SetUserAuthInRequest(r, userAuth)
	return nil
}

// CheckPATFromRequest checks if the PAT is valid
func (m *AuthMiddleware) checkPATFromRequest(r *http.Request, authHeaderParts []string) error {
	token, err := getTokenFromPATRequest(authHeaderParts)
	if err != nil {
		return fmt.Errorf("error extracting token: %w", err)
	}

	if m.patUsageTracker != nil {
		m.patUsageTracker.IncrementUsage(token)
	}

	if !isTerraformRequest(r) && !m.rateLimiter.Allow(token) {
		return status.Errorf(status.TooManyRequests, "too many requests")
	}

	ctx := r.Context()
	user, pat, accDomain, accCategory, err := m.authManager.GetPATInfo(ctx, token)
	if err != nil {
		return fmt.Errorf("invalid Token: %w", err)
	}
	if time.Now().After(pat.GetExpirationDate()) {
		return fmt.Errorf("token expired")
	}

	if pat.LastUsed == nil || time.Since(*pat.LastUsed) >= types.PATLastUsedUpdateInterval {
		if err := m.authManager.MarkPATUsed(ctx, pat.ID); err != nil {
			return err
		}
	}

	userAuth := auth.UserAuth{
		UserId:         user.Id,
		AccountId:      user.AccountID,
		Domain:         accDomain,
		DomainCategory: accCategory,
		IsPAT:          true,
	}

	// propagates ctx change to upstream middleware
	*r = *nbcontext.SetUserAuthInRequest(r, userAuth)
	return nil
}

func isTerraformRequest(r *http.Request) bool {
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	return strings.Contains(ua, "terraform")
}

// getTokenFromJWTRequest is a "TokenExtractor" that takes auth header parts and extracts
// the JWT token from the Authorization header.
func getTokenFromJWTRequest(authHeaderParts []string) (string, error) {
	if len(authHeaderParts) != 2 || strings.ToLower(authHeaderParts[0]) != "bearer" {
		return "", errors.New("authorization header format must be Bearer {token}")
	}

	return authHeaderParts[1], nil
}

// getTokenFromPATRequest is a "TokenExtractor" that takes auth header parts and extracts
// the PAT token from the Authorization header.
func getTokenFromPATRequest(authHeaderParts []string) (string, error) {
	if len(authHeaderParts) != 2 || strings.ToLower(authHeaderParts[0]) != "token" {
		return "", errors.New("authorization header format must be Token {token}")
	}

	return authHeaderParts[1], nil
}
