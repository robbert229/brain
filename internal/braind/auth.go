package braind

// Authentication wiring for the daemon HTTP surface (spec 002). The sign-in
// handlers are registered only when OIDC is enabled; probes, status, static
// assets, and the landing page stay public because Kubernetes and operators
// must reach them without an identity provider round trip (ADR-0003).

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/johnrowl/brain/internal/braind/auth"
	"github.com/johnrowl/brain/internal/braind/config"
	"github.com/johnrowl/brain/internal/braind/status"
)

// discoveryRefreshInterval bounds how stale the operator-visible discovery state
// and provider health view may get. Request paths never wait on this ticker; a
// degraded provider fails closed through the reauthentication gate instead
// (FR-012, FR-013).
const discoveryRefreshInterval = 30 * time.Second

// authSurface owns the OIDC pieces of the daemon. Sessions live in memory for
// the process lifetime (research D2), so a restart signs everyone out.
type authSurface struct {
	handler   *auth.Handler
	sessions  *auth.SessionStore
	provider  *auth.Provider
	done      chan struct{}
	closeOnce sync.Once
}

// newAuthSurface builds the sign-in surface, returning nil when OIDC is
// disabled. Configuration the auth package rejects is reported as an error so
// run() can serve the public surface instead of failing to start.
func newAuthSurface(processConfig config.Config, logger *slog.Logger) (*authSurface, error) {
	oidc := processConfig.OIDC()
	if !oidc.Enabled {
		return nil, nil
	}
	sessionPolicy := processConfig.Session()
	baseURL := strings.TrimRight(processConfig.Server().ExternalURL, "/")
	provider, err := auth.NewProvider(auth.ProviderConfig{
		ConfigurationURL: oidc.ConfigurationURL,
		ClientID:         oidc.ClientID,
		ClientSecret:     oidc.ClientSecret,
		RedirectURL:      baseURL + "/oauth2/callback",
	})
	if err != nil {
		return nil, err
	}
	sessions := auth.NewSessionStore(auth.SessionPolicy{
		IdleTimeout:      sessionPolicy.IdleTimeout,
		AbsoluteLifetime: sessionPolicy.AbsoluteLifetime,
		ReauthInterval:   sessionPolicy.ReauthInterval,
	}, nil)
	handler, err := auth.NewHandler(auth.HandlerConfig{
		Provider:     provider,
		Sessions:     sessions,
		Transactions: auth.NewTransactionStore(auth.DefaultTransactionTTL, nil),
		Roles: auth.RoleConfig{
			AdminGroup:  oidc.AdminGroup,
			UserGroup:   oidc.UserGroup,
			ViewerGroup: oidc.ViewerGroup,
		},
		GroupsClaim:  oidc.GroupsClaim,
		UserClaim:    oidc.UserClaim,
		GroupsSource: oidc.GroupsSource,
		AutoRedirect: oidc.AutoRedirect,
		BaseURL:      baseURL,
		Logger:       logger.With("component", "auth"),
		// The groups scope is how the provider returns the claim the role map
		// reads; OIDC_EXTRA_SCOPES extends it without dropping what matters.
		Scopes: append([]string{"openid", "profile", "email", "groups"}, oidc.ExtraScopes...),
	})
	if err != nil {
		return nil, err
	}
	sessions.StartCleanup()
	return &authSurface{
		handler:  handler,
		sessions: sessions,
		provider: provider,
		done:     make(chan struct{}),
	}, nil
}

// register mounts the sign-in surface. The anonymous entry points and the
// policy-gated routes are registered separately so spec 003+ can add protected
// vault and API routes through Handler.RequireAuth without touching probes,
// status, or the landing page.
func (s *authSurface) register(mux *http.ServeMux) {
	if s == nil {
		return
	}
	s.handler.RegisterPublic(mux)
	s.handler.RegisterProtected(mux)
}

// watch performs the first discovery attempt off the startup path and then keeps
// provider health and the status snapshot truthful. Startup never waits on the
// provider: an outage here changes nothing about readiness (FR-013).
func (s *authSurface) watch(ctx context.Context, publisher *status.Store) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.refresh(ctx, publisher)
	ticker := time.NewTicker(discoveryRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-ticker.C:
			s.refresh(ctx, publisher)
		}
	}
}

func (s *authSurface) refresh(ctx context.Context, publisher *status.Store) {
	requestContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := s.provider.Refresh(requestContext); err != nil {
		publisher.Update(func(snapshot *status.Snapshot) {
			snapshot.OIDC.Discovery = status.OIDCDegraded
		})
		return
	}
	publisher.Update(func(snapshot *status.Snapshot) {
		snapshot.OIDC.Discovery = status.OIDCReady
		snapshot.OIDC.ActiveSessions = s.sessions.Count()
	})
}

// publishSessions keeps active_sessions accurate from the status path rather
// than from request handling (contracts/http.md).
func (s *authSurface) publishSessions(publisher *status.Store) {
	if s == nil {
		return
	}
	s.sessions.Prune()
	publisher.Update(func(snapshot *status.Snapshot) {
		snapshot.OIDC.ActiveSessions = s.sessions.Count()
	})
}

// Close stops the discovery watch and the session sweeper.
func (s *authSurface) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		close(s.done)
		s.sessions.StopCleanup()
	})
}
