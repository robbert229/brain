package auth

import (
	"context"
	"net/http"
	"net/url"
)

// Policy is the access requirement one protected route declares. Minimum is the
// lowest role allowed to reach the handler; one policy answers browser and JSON
// surfaces identically (FR-006, FR-010).
type Policy struct {
	Minimum Role
}

// RequireAuth wraps next with the single authentication and authorization
// decision every protected surface shares. It classifies the request once, then
// either continues with the verified identity in context or answers with the
// outcome the caller expects (contracts/http.md).
func (h *Handler) RequireAuth(policy Policy, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, outcome := h.LookupSession(r)
		switch outcome {
		case OutcomeActive:
			if isStateChange(r) && !validCSRF(w, r, session.CSRFToken) {
				h.config.Logger.Warn("request rejected", "component", "auth", "outcome", "csrf_check_failed")
				h.respondFailure(w, r, http.StatusForbidden, "csrf_check_failed")
				return
			}
		case OutcomeReauthRequired:
			// The reauthentication interval is the only place cached
			// authorization is allowed. With a reachable provider the visitor
			// signs in again; without one the request fails closed and the
			// session is never extended (FR-012, SC-004).
			if !h.config.Provider.Healthy() {
				h.respondFailure(w, r, http.StatusServiceUnavailable, "authentication_unavailable")
				return
			}
			h.deny(w, r, http.StatusUnauthorized, "reauthentication_required")
			return
		default:
			h.deny(w, r, http.StatusUnauthorized, "unauthenticated")
			return
		}

		if !session.Role.Meets(policy.Minimum) {
			h.config.Logger.Warn("request denied", "component", "auth", "outcome", "forbidden", "role", string(session.Role))
			h.respondFailure(w, r, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, ContextWithIdentity(r, session.Identity()))
	})
}

// RegisterPublic mounts the anonymous sign-in entry points.
func (h *Handler) RegisterPublic(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", h.handleLogin)
	mux.HandleFunc("GET /oauth2/callback", h.handleCallback)
}

// RegisterProtected mounts routes that require an authenticated session. In
// spec 002 that is only sign-out, whose policy is any verified role; vault and
// API routes from later specs mount the same way so they inherit this decision.
func (h *Handler) RegisterProtected(mux *http.ServeMux) {
	mux.Handle("POST /logout", h.RequireAuth(Policy{}, http.HandlerFunc(h.handleLogout)))
}

// Register mounts the full sign-in surface: anonymous entry points plus the
// policy-gated routes.
func (h *Handler) Register(mux *http.ServeMux) {
	h.RegisterPublic(mux)
	h.RegisterProtected(mux)
}

// deny answers an authorization gap. Browsers recover by signing in, so they get
// a redirect carrying where they wanted to go; API clients get the JSON outcome.
func (h *Handler) deny(w http.ResponseWriter, r *http.Request, status int, code string) {
	if wantsJSON(r) {
		h.respondFailure(w, r, status, code)
		return
	}
	h.config.Logger.Info("authorization required", "component", "auth", "outcome", code)
	http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.Path), http.StatusFound)
}

// isStateChange reports whether the request can mutate state, which is what
// decides when the CSRF check applies (FR-008).
func isStateChange(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

// identityContextKey keys the verified identity inside a request context.
type identityContextKey struct{}

// ContextWithIdentity returns a request carrying the verified identity so
// downstream handlers never read cookies or tokens themselves.
func ContextWithIdentity(r *http.Request, identity Identity) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identityContextKey{}, identity))
}

// FromContext returns the identity an authenticated request carries, reporting
// false for anonymous requests so later specs fail closed rather than guess.
func FromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(Identity)
	if !ok || identity.Subject == "" {
		return Identity{}, false
	}
	return identity, true
}

// Identity builds the verified identity a session remembers (data-model.md).
func (s Session) Identity() Identity {
	return Identity{
		Subject:    s.Subject,
		UserClaim:  s.UserClaim,
		Groups:     append([]string(nil), s.Groups...),
		Role:       s.Role,
		VerifiedAt: s.LastVerifiedAt,
	}
}
