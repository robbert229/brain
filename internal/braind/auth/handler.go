package auth

import (
	"crypto/subtle"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// CookieName is the sole browser-visible session identifier. It carries nothing
// but its own value: tokens and claims stay server-side (FR-003).
const CookieName = "braind_session"

const (
	// CSRFHeader is the required header for state-changing requests (FR-008).
	CSRFHeader = "X-CSRF-Token"
	// CSRFFormField is the form field name accepted for progressive enhancement.
	CSRFFormField = "csrf_token"
)

// webAssets holds the sign-in page templates. Embedding keeps the binary
// self-contained and uses the same mechanism as the landing page
// (landing.go): parsed once at init, rendered to text/html, and styled by the
// shared /static/style.css the daemon serves.
//
//go:embed webassets
var webAssets embed.FS

var (
	loginTemplate = template.Must(template.ParseFS(webAssets, "webassets/login.html.tmpl"))
	// failureTemplate renders browser-facing failure pages; the JSON shape of
	// the same outcome is served instead to API callers (respondFailure).
	failureTemplate = template.Must(template.ParseFS(webAssets, "webassets/failure.html.tmpl"))
)

// failurePage is the complete data context for failureTemplate.
type failurePage struct {
	Title string
}

// HandlerConfig configures the sign-in HTTP surface.
type HandlerConfig struct {
	Provider     *Provider
	Sessions     *SessionStore
	Transactions *TransactionStore
	Roles        RoleConfig
	GroupsClaim  string
	UserClaim    string
	GroupsSource string
	AutoRedirect bool
	BaseURL      string
	Logger       *slog.Logger
	Scopes       []string
	Clock        func() time.Time
}

// Handler serves /login, /oauth2/callback, and /logout.
type Handler struct {
	config HandlerConfig
}

var (
	errStateMissing = errors.New("state is required")
)

// NewHandler validates the configuration and prepares the sign-in pages.
func NewHandler(config HandlerConfig) (*Handler, error) {
	if config.Provider == nil || config.Sessions == nil || config.Transactions == nil {
		return nil, errors.New("auth: provider, sessions, and transactions are required")
	}
	if config.GroupsClaim == "" {
		config.GroupsClaim = "groups"
	}
	if config.UserClaim == "" {
		config.UserClaim = "email"
	}
	if config.GroupsSource == "" {
		config.GroupsSource = "id_token"
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if _, err := url.Parse(config.BaseURL); err != nil {
		return nil, fmt.Errorf("auth: base url: %w", err)
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &Handler{config: config}, nil
}

// handleLogin issues a login transaction and either renders the sign-in prompt
// or redirects straight to the provider.
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	// A signed-in visitor reaching /login followed a stale bookmark, the back
	// button, or a leftover ?login_error from an earlier attempt; send them on
	// to where they meant to go instead of offering a second sign-in.
	if _, outcome := h.LookupSession(r); outcome == OutcomeActive {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, SafeRedirect(r.URL.Query().Get("next")), http.StatusFound)
		return
	}
	transaction, ok := h.config.Transactions.Issue(r.URL.Query().Get("next"))
	if !ok {
		h.respondFailure(w, r, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	authorizeURL, err := h.config.Provider.AuthorizeURL(r.Context(), transaction.State, transaction.Nonce, transaction.Verifier, h.config.Scopes)
	if err != nil {
		h.config.Logger.Warn("sign-in start failed", "component", "auth", "outcome", "discovery_unavailable")
		h.respondFailure(w, r, http.StatusServiceUnavailable, "authentication_unavailable")
		return
	}
	if h.config.AutoRedirect {
		http.Redirect(w, r, authorizeURL, http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'")
	if err := loginTemplate.Execute(w, struct {
		AuthorizeURL string
		Error        string
	}{AuthorizeURL: authorizeURL, Error: sanitizedError(r.URL.Query().Get("login_error"))}); err != nil {
		h.config.Logger.Error("render sign-in prompt", "component", "auth", "error", err)
	}
}

// handleCallback completes the authorization-code flow. Every failure path
// creates no session (FR-002, SC-002).
func (h *Handler) handleCallback(w http.ResponseWriter, r *http.Request) {
	queries := r.URL.Query()
	if providerError := queries.Get("error"); providerError != "" {
		h.config.Logger.Warn("provider rejected sign-in", "component", "auth", "outcome", "provider_error", "code", sanitizedError(providerError))
		http.Redirect(w, r, "/login?login_error="+url.QueryEscape(sanitizedError(providerError)), http.StatusSeeOther)
		return
	}
	code := queries.Get("code")
	if code == "" {
		h.respondFailure(w, r, http.StatusBadRequest, "invalid_login_transaction")
		return
	}
	transaction, ok := h.config.Transactions.Redeem(queries.Get("state"))
	if !ok {
		h.config.Logger.Warn("sign-in rejected", "component", "auth", "outcome", "invalid_login_transaction")
		h.respondFailure(w, r, http.StatusBadRequest, "invalid_login_transaction")
		return
	}

	tokens, err := h.config.Provider.Exchange(r.Context(), code, transaction.Verifier, transaction.Nonce)
	if err != nil {
		h.config.Logger.Warn("token exchange failed", "component", "auth", "outcome", "provider_unavailable", "error", redactedError(err))
		http.Redirect(w, r, "/login?login_error=provider_unavailable", http.StatusSeeOther)
		return
	}

	claims := tokens.Claims
	if h.config.GroupsSource == "userinfo" {
		userinfo, err := h.config.Provider.Userinfo(r.Context(), tokens.AccessToken)
		if err != nil {
			h.config.Logger.Warn("userinfo lookup failed", "component", "auth", "outcome", "provider_unavailable", "error", redactedError(err))
			h.respondFailure(w, r, http.StatusServiceUnavailable, "authentication_unavailable")
			return
		}
		claims = mergeGroupsClaims(claims, userinfo, h.config.GroupsClaim)
	}

	groups, ok := GroupsFromClaim(claims[h.config.GroupsClaim])
	if !ok {
		h.config.Logger.Warn("group claim unusable", "component", "auth", "outcome", "malformed_groups")
		h.respondFailure(w, r, http.StatusForbidden, "access_denied")
		return
	}
	role := ResolveRole(groups, h.config.Roles)
	if role == RoleNone {
		h.config.Logger.Warn("identity has no configured role", "component", "auth", "outcome", "access_denied")
		h.respondFailure(w, r, http.StatusForbidden, "access_denied")
		return
	}

	subject, _ := claims["sub"].(string)
	userClaim, _ := claims[h.config.UserClaim].(string)
	identity := Identity{Subject: subject, UserClaim: userClaim, Groups: groups, VerifiedAt: h.config.Clock()}

	var session Session
	if existing := h.currentSession(r); existing.ID != "" {
		session, err = h.config.Sessions.Rotate(existing.ID, identity, role)
	} else {
		session, err = h.config.Sessions.Create(identity, role)
	}
	if err != nil {
		h.config.Logger.Error("session creation failed", "component", "auth", "error", err)
		h.respondFailure(w, r, http.StatusInternalServerError, "internal_error")
		return
	}
	setSessionCookie(w, session.ID)
	h.config.Logger.Info("sign-in completed", "component", "auth", "outcome", "granted", "role", string(role))
	http.Redirect(w, r, transaction.PostLoginRedirect, http.StatusFound)
}

// handleLogout destroys the local session and, when the provider advertises an
// end-session endpoint, ends the provider session too (US1-AC3).
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	session := h.currentSession(r)
	if session.ID == "" {
		// Nothing to end; clear any stale cookie and finish quietly.
		clearSessionCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !validCSRF(w, r, session.CSRFToken) {
		h.config.Logger.Warn("logout rejected", "component", "auth", "outcome", "csrf_check_failed")
		h.respondFailure(w, r, http.StatusForbidden, "csrf_check_failed")
		return
	}
	h.config.Sessions.Destroy(session.ID)
	clearSessionCookie(w)

	// Sending a browser to a provider that is known to be failing would replace
	// a completed local sign-out with an error page, so the redirect happens only
	// while the provider is reachable (US1-AC3 edge case).
	endSession, advertised := h.config.Provider.EndSessionURL()
	if !advertised || !h.config.Provider.Healthy() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	redirect := endSession + "?post_logout_redirect_uri=" + url.QueryEscape(h.config.BaseURL+"/")
	http.Redirect(w, r, redirect, http.StatusFound)
}

// currentSession resolves the session named by the request cookie, ignoring the
// outcome so callers that only need "is there a session" can read it cheaply.
func (h *Handler) currentSession(r *http.Request) Session {
	session, _ := h.LookupSession(r)
	return session
}

// LookupSession resolves a session and reports its outcome. Middleware uses it so
// one lookup drives both policy and reauth decisions.
func (h *Handler) LookupSession(r *http.Request) (Session, SessionOutcome) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return Session{}, OutcomeExpired
	}
	return h.config.Sessions.Lookup(cookie.Value)
}

// CurrentSession exposes the request session to the daemon's middleware.
func (h *Handler) CurrentSession(r *http.Request) (Session, SessionOutcome) {
	return h.LookupSession(r)
}

// validCSRF accepts the session token from either the header or form field.
const maxCSRFMultipartBytes = 16 << 20

func validCSRF(w http.ResponseWriter, r *http.Request, token string) bool {
	candidate := r.Header.Get(CSRFHeader)
	if candidate == "" {
		var err error
		if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data") {
			r.Body = http.MaxBytesReader(w, r.Body, maxCSRFMultipartBytes)
			err = r.ParseMultipartForm(1 << 20)
		} else {
			err = r.ParseForm()
		}
		if err == nil {
			candidate = r.Form.Get(CSRFFormField)
		}
	}
	if candidate == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) == 1
}

func setSessionCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// respondFailure writes the outcome code in the shape the caller expects,
// revealing nothing beyond the classification itself (FR-009).
func (h *Handler) respondFailure(w http.ResponseWriter, r *http.Request, status int, code string) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, `{"error":%q}`, code)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'")
	w.WriteHeader(status)
	_ = failureTemplate.Execute(w, failurePage{Title: failureTitle(code)})
}

// mergeGroupsClaims prefers the configured group source while keeping identity
// claims from the ID token, so GroupsSource changes what authorizes, not who
// signed in (FR-004).
func mergeGroupsClaims(idClaims, userinfoClaims map[string]any, groupsClaim string) map[string]any {
	merged := make(map[string]any, len(idClaims)+1)
	for name, value := range idClaims {
		merged[name] = value
	}
	if value, ok := userinfoClaims[groupsClaim]; ok {
		merged[groupsClaim] = value
	}
	return merged
}

func failureTitle(code string) string {
	switch code {
	case "access_denied", "forbidden":
		return "Access denied"
	case "csrf_check_failed":
		return "Request rejected"
	case "unauthenticated", "reauthentication_required":
		return "Sign-in required"
	case "authentication_unavailable":
		return "Sign-in unavailable"
	default:
		return "Sign-in failed"
	}
}

// wantsJSON keeps one outcome vocabulary for browser and API surfaces (FR-006).
func wantsJSON(r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		return true
	}
	accept := r.Header.Get("Accept")
	if accept == "" {
		return false
	}
	return strings.Contains(accept, "application/json")
}

// sanitizedError keeps only a coarse provider code so a callback cannot smuggle
// text into the response (FR-009).
func sanitizedError(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "provider_error"
	}
	if len(value) > 48 {
		return "provider_error"
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && character != '_' && character != '-' {
			return "provider_error"
		}
	}
	return value
}

// tokenPattern matches JWT-shaped values (base64url segments starting with
// the '{"' JSON prefix) and long opaque runs that could be secrets, so
// library-supplied error text can never smuggle token material into a log
// line (FR-009).
var tokenPattern = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{5,}(?:\.[A-Za-z0-9_-]*){2}|[A-Za-z0-9_-]{40,}`)

// redactedError strips anything that looks like a token or credential from error
// text before it reaches logs. Errors the auth package builds itself carry only
// coarse, safe detail, so the whole message survives; the scrubbing guards
// against provider text echoing secrets. Truncation keeps log lines bounded —
// it must not become the reason a diagnostic disappears, so no positional
// cutting is applied.
func redactedError(err error) string {
	if err == nil {
		return ""
	}
	message := tokenPattern.ReplaceAllString(err.Error(), "[redacted]")
	if len(message) > 200 {
		message = message[:200]
	}
	return message
}
