package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Protocol mechanics live in coreos/go-oidc and golang.org/x/oauth2
// (ADR-0010): discovery validation, JWKS fetch and rotation, signature
// verification, standard-claim checks, PKCE, and token exchange. What remains
// here is braind-specific: health bookkeeping for the status snapshot (FR-013),
// the endpoint-origin hardening beyond library defaults, and the coarse error
// vocabulary that keeps token material out of logs (FR-009).

// DiscoveryState is the operator-visible view of provider availability.
type DiscoveryState string

const (
	DiscoveryPending  DiscoveryState = "pending"
	DiscoveryReady    DiscoveryState = "ready"
	DiscoveryDegraded DiscoveryState = "degraded"
)

// HealthState is the coarse view middleware consults before failing a request
// closed during an outage.
type HealthState string

const (
	HealthHealthy  HealthState = "healthy"
	HealthDegraded HealthState = "degraded"
)

// ProviderConfig configures one OIDC provider (the only supported configuration,
// per ADR-0005).
type ProviderConfig struct {
	ConfigurationURL string
	ClientID         string
	ClientSecret     string
	RedirectURL      string
	ExtraScopes      []string
	HTTPClient       *http.Client
	Now              func() time.Time
}

// TokenSet is the outcome of a code exchange with a verified ID token.
type TokenSet struct {
	AccessToken string
	Claims      map[string]any
}

// Provider performs discovery, signature-verified ID-token validation, and code
// exchange. Tokens and claims are held in memory only, never logged (FR-003).
type Provider struct {
	config ProviderConfig
	issuer string

	mu         sync.Mutex
	endpoints  discoveryDocument
	oidc       *oidc.Provider
	verifier   *oidc.IDTokenVerifier
	state      DiscoveryState
	lastHealth time.Time
}

// discoveryDocument keeps the fields the library does not expose or that
// braind hardens beyond library defaults: end_session_endpoint for sign-out
// (US1-AC3), PKCE and flow advertisement, and endpoint-origin validation.
type discoveryDocument struct {
	Issuer                        string   `json:"issuer"`
	AuthorizationEndpoint         string   `json:"authorization_endpoint"`
	TokenEndpoint                 string   `json:"token_endpoint"`
	UserinfoEndpoint              string   `json:"userinfo_endpoint"`
	JWKSURI                       string   `json:"jwks_uri"`
	EndSessionEndpoint            string   `json:"end_session_endpoint"`
	ResponseTypesSupported        []string `json:"response_types_supported"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
}

// NewProvider validates configuration shape without contacting the provider, so
// startup succeeds even while the provider is down (FR-013).
func NewProvider(config ProviderConfig) (*Provider, error) {
	issuer, err := issuerFromConfigurationURL(config.ConfigurationURL)
	if err != nil {
		return nil, err
	}
	if config.ClientID == "" {
		return nil, errors.New("oidc: client id is required")
	}
	if config.RedirectURL == "" {
		return nil, errors.New("oidc: redirect url is required")
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Provider{
		config: config,
		issuer: issuer,
		state:  DiscoveryPending,
	}, nil
}

// Refresh fetches discovery through go-oidc and confirms the signing keys are
// reachable. It records the outcome and returns the error: callers on request
// paths MUST treat failure as degraded, never as fatal, and readiness MUST NOT
// consult it (FR-013, ADR-0003). The next successful call replaces the
// verifier, which is what makes provider key rotation survivable.
func (p *Provider) Refresh(ctx context.Context) error {
	providerCtx := oidc.ClientContext(ctx, p.config.HTTPClient)
	// go-oidc performs discovery and enforces issuer equality; a failure here
	// means the provider cannot be trusted, so nothing is swapped in.
	rp, err := oidc.NewProvider(providerCtx, p.issuer)
	if err != nil {
		p.mark(errors.New("discovery failed"))
		return errors.New("oidc: discovery request failed")
	}
	document, err := p.fetchDiscovery(ctx)
	if err != nil {
		p.mark(err)
		return err
	}
	if err := p.validateDiscovery(document); err != nil {
		p.mark(err)
		return err
	}
	if err := p.checkKeysReachable(ctx, document.JWKSURI); err != nil {
		p.mark(err)
		return err
	}
	p.mu.Lock()
	p.endpoints = document
	p.oidc = rp
	p.verifier = rp.Verifier(&oidc.Config{ClientID: p.config.ClientID, Now: p.config.Now})
	p.state = DiscoveryReady
	p.lastHealth = p.config.Now()
	p.mu.Unlock()
	return nil
}

// DiscoveryState reports discovery health for the status snapshot.
func (p *Provider) DiscoveryState() DiscoveryState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// Health reports whether provider round trips are succeeding. Callers gate on
// this instead of the discovery timestamp so a recovered provider is trusted
// again immediately (data-model.md ProviderHealth).
func (p *Provider) Health() HealthState {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state == DiscoveryReady {
		return HealthHealthy
	}
	return HealthDegraded
}

// Healthy reports whether the last provider round trip succeeded.
func (p *Provider) Healthy() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state == DiscoveryReady
}

// Issuer returns the validated issuer.
func (p *Provider) Issuer() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.issuer
}

// AuthorizeURL builds the provider authorization request using S256 PKCE
// through golang.org/x/oauth2. When the last provider round trip failed it
// makes one discovery attempt so a recovered provider is usable again without a
// background poller; a still-failing provider returns an error, which the
// handler reports as authentication_unavailable (FR-012, FR-013).
func (p *Provider) AuthorizeURL(ctx context.Context, state, nonce, verifier string, scopes []string) (string, error) {
	if !p.Healthy() {
		if err := p.Refresh(ctx); err != nil {
			return "", err
		}
	}
	p.mu.Lock()
	endpoints := p.endpoints
	p.mu.Unlock()
	if endpoints.AuthorizationEndpoint == "" {
		return "", errors.New("oidc: provider discovery is unavailable")
	}
	if !contains(endpoints.CodeChallengeMethodsSupported, "S256") {
		return "", errors.New("oidc: provider does not advertise S256 PKCE")
	}
	if !contains(endpoints.ResponseTypesSupported, "code") {
		return "", errors.New("oidc: provider does not support the authorization code flow")
	}
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email", "groups"}
	}
	config := p.oauth2Config(endpoints)
	// Without Scopes on the config, AuthCodeURL omits the scope parameter, the
	// provider issues a non-openid code, and the token response carries no
	// id_token at all. The config is rebuilt per call, so this is local.
	config.Scopes = scopes
	authorizeURL := config.AuthCodeURL(state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.S256ChallengeOption(verifier),
	)
	// Values.Encode writes spaces as '+', which the prompt page escapes into
	// the HTML entity form and any URL parser then reads as a fragment start.
	// The scope list is space-separated (RFC 6749), so write its separators as
	// %20. Every other query value here is base64url and carries no '+'.
	parsed, err := url.Parse(authorizeURL)
	if err != nil {
		return "", fmt.Errorf("oidc: authorization endpoint: %w", err)
	}
	parsed.RawQuery = strings.ReplaceAll(parsed.RawQuery, "+", "%20")
	return parsed.String(), nil
}

// EndSessionURL returns the provider logout endpoint when discovery advertised
// one (US1-AC3); otherwise false means local-only sign-out.
func (p *Provider) EndSessionURL() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.endpoints.EndSessionEndpoint == "" {
		return "", false
	}
	return p.endpoints.EndSessionEndpoint, true
}

// Exchange redeems an authorization code and returns verified ID-token claims.
// Signature, issuer, audience, expiry, and nonce are all enforced here (FR-002).
func (p *Provider) Exchange(ctx context.Context, code, verifier, nonce string) (*TokenSet, error) {
	p.mu.Lock()
	endpoints := p.endpoints
	p.mu.Unlock()
	if endpoints.TokenEndpoint == "" {
		return nil, errors.New("oidc: provider discovery is unavailable")
	}
	config := p.oauth2Config(endpoints)
	token, err := config.Exchange(oidc.ClientContext(ctx, p.config.HTTPClient), code, oauth2.VerifierOption(verifier))
	if err != nil {
		// The library error can echo provider response text; surface only the
		// safe shape of it (status plus registered OAuth error code, FR-009).
		p.mark(errors.New("token endpoint error"))
		return nil, exchangeFailure(err)
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	if rawIDToken == "" {
		p.mark(errors.New("missing id_token"))
		return nil, fmt.Errorf("oidc: provider returned no ID token (access_token=%v refresh_token=%v)", token.AccessToken != "", token.RefreshToken != "")
	}
	claims, err := p.VerifyIDToken(ctx, rawIDToken, nonce)
	if err != nil {
		return nil, err
	}
	p.mark(nil)
	return &TokenSet{AccessToken: token.AccessToken, Claims: claims}, nil
}

// Userinfo fetches the userinfo document with a bearer access token, used when
// OIDC_GROUPS_SOURCE=userinfo (FR-004).
func (p *Provider) Userinfo(ctx context.Context, accessToken string) (map[string]any, error) {
	p.mu.Lock()
	rp := p.oidc
	endpoint := p.endpoints.UserinfoEndpoint
	p.mu.Unlock()
	if endpoint == "" {
		return nil, errors.New("oidc: provider has no userinfo endpoint")
	}
	if rp == nil {
		return nil, errors.New("oidc: discovery is not loaded")
	}
	source := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	userInfo, err := rp.UserInfo(oidc.ClientContext(ctx, p.config.HTTPClient), source)
	if err != nil {
		p.mark(errors.New("userinfo error"))
		return nil, errors.New("oidc: userinfo request failed")
	}
	var claims map[string]any
	if err := userInfo.Claims(&claims); err != nil {
		p.mark(errors.New("malformed userinfo response"))
		return nil, errors.New("oidc: malformed userinfo response")
	}
	p.mark(nil)
	return claims, nil
}

// VerifyIDToken enforces every claim check that must pass before a session is
// created (FR-002). go-oidc verifies the signature against the discovered JWKS
// (refreshing once for an unknown kid) and checks issuer, audience, expiry,
// and issue time; nonce comparison stays ours because the library leaves it
// to the caller. Error text names the violated rule but never echoes token
// material (FR-009).
func (p *Provider) VerifyIDToken(ctx context.Context, token, nonce string) (map[string]any, error) {
	p.mu.Lock()
	verifier := p.verifier
	p.mu.Unlock()
	if verifier == nil {
		return nil, errors.New("oidc: discovery is not loaded")
	}
	idToken, err := verifier.Verify(oidc.ClientContext(ctx, p.config.HTTPClient), token)
	if err != nil {
		return nil, fmt.Errorf("oidc: id token rejected: %s", redactedError(err))
	}
	if idToken.Nonce != nonce {
		return nil, errors.New("oidc: id token nonce mismatch")
	}
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return nil, errors.New("oidc: id token payload is not JSON")
	}
	subject, ok := claims["sub"].(string)
	if !ok || subject == "" {
		return nil, errors.New("oidc: id token has no subject")
	}
	return claims, nil
}

// ChallengeFromVerifier derives the S256 code challenge from a verifier. The
// OAuth2 library derives it internally for authorize requests; this export
// exists so tests can build provider authorize URLs directly.
func ChallengeFromVerifier(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (p *Provider) oauth2Config(endpoints discoveryDocument) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     p.config.ClientID,
		ClientSecret: p.config.ClientSecret,
		RedirectURL:  p.config.RedirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:   endpoints.AuthorizationEndpoint,
			TokenURL:  endpoints.TokenEndpoint,
			AuthStyle: oauth2.AuthStyleInHeader,
		},
	}
}

// exchangeFailure maps an x/oauth2 exchange error onto the coarse, token-free
// vocabulary logs may carry (FR-009). The library pre-parses the registered
// OAuth error code — safe even when an unorthodox provider answers an error
// with HTTP 200 — while the raw response body, which can carry token
// material, is never copied into the message.
func exchangeFailure(err error) error {
	var retrieve *oauth2.RetrieveError
	if errors.As(err, &retrieve) {
		status := 0
		if retrieve.Response != nil {
			status = retrieve.Response.StatusCode
		}
		if code := sanitizeErrorCode(retrieve.ErrorCode); code != "" {
			return fmt.Errorf("oidc: token endpoint returned HTTP %d (%s)", status, code)
		}
		return fmt.Errorf("oidc: token endpoint returned HTTP %d", status)
	}
	if strings.Contains(err.Error(), "missing access_token") {
		return errors.New("oidc: token endpoint answered without an access token")
	}
	return errors.New("oidc: token exchange failed")
}

// sanitizeErrorCode keeps the OAuth error field only in its registered wire
// form: RFC 6749 names are alphanumeric plus underscore. Anything else is
// treated as provider free text and dropped.
func sanitizeErrorCode(value string) string {
	if value == "" || len(value) > 40 {
		return ""
	}
	for _, r := range value {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return ""
	}
	return value
}

// checkKeysReachable proves the JWKS endpoint answers with at least one key.
// Actual key selection and caching belong to go-oidc.
func (p *Provider) checkKeysReachable(ctx context.Context, uri string) error {
	var document struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := p.getJSON(ctx, uri, "", &document); err != nil {
		return err
	}
	if len(document.Keys) == 0 {
		return errors.New("oidc: jwks document contains no keys")
	}
	return nil
}

func (p *Provider) fetchDiscovery(ctx context.Context) (discoveryDocument, error) {
	var document discoveryDocument
	if err := p.getJSON(ctx, p.config.ConfigurationURL, "", &document); err != nil {
		return document, err
	}
	return document, nil
}

func (p *Provider) getJSON(ctx context.Context, endpoint, bearer string, destination any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := p.config.HTTPClient.Do(request)
	if err != nil {
		return errors.New("oidc: provider request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("oidc: provider returned HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(destination); err != nil {
		return errors.New("oidc: provider returned malformed JSON")
	}
	return nil
}

func (p *Provider) validateDiscovery(document discoveryDocument) error {
	if document.Issuer != p.issuer {
		return fmt.Errorf("oidc: discovery issuer %q does not match configured issuer", document.Issuer)
	}
	if !contains(document.ResponseTypesSupported, "code") {
		return errors.New("oidc: provider does not advertise the authorization code flow")
	}
	for name, endpoint := range map[string]string{
		"authorization_endpoint": document.AuthorizationEndpoint,
		"token_endpoint":         document.TokenEndpoint,
		"jwks_uri":               document.JWKSURI,
	} {
		if endpoint == "" {
			return fmt.Errorf("oidc: discovery is missing %s", name)
		}
		if err := sameOriginOrLoopback(p.issuer, endpoint); err != nil {
			return fmt.Errorf("oidc: %s: %w", name, err)
		}
	}
	return nil
}

// mark records the outcome of a provider round trip for health reporting.
func (p *Provider) mark(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastHealth = p.config.Now()
	if err == nil {
		p.state = DiscoveryReady
		return
	}
	p.state = DiscoveryDegraded
}

func issuerFromConfigurationURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("oidc: configuration url is invalid: %w", err)
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return "", errors.New("oidc: issuer must be HTTPS")
	}
	if parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("oidc: configuration url must be an absolute URL without query or fragment")
	}
	// An OIDC issuer never carries a trailing slash, so dropping the well-known
	// suffix (and any single trailing slash) yields the canonical issuer form.
	path := strings.TrimSuffix(parsed.Path, "/")
	path = strings.TrimSuffix(path, "/.well-known/openid-configuration")
	return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host, Path: path}).String(), nil
}

func sameOriginOrLoopback(issuer, endpoint string) error {
	issuerURL, err := url.Parse(issuer)
	if err != nil {
		return err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	if parsed.Scheme == "https" {
		if parsed.Host != issuerURL.Host {
			return errors.New("must be on the issuer host")
		}
		return nil
	}
	// Loopback HTTP is accepted only so the CI simulated provider can run
	// without certificates; remote issuers are always HTTPS (FR-001).
	if parsed.Scheme == "http" && (isLoopbackHost(parsed.Hostname()) && isLoopbackHost(issuerURL.Hostname())) {
		return nil
	}
	return errors.New("must be HTTPS")
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]"
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
