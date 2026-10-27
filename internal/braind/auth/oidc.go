package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DiscoveryState is the operator-visible view of provider availability.
type DiscoveryState string

const (
	DiscoveryDisabled DiscoveryState = "disabled"
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

// Disabled is reported by the status snapshot, not by a live provider, so
// HealthState has no disabled member: an unconfigured provider has no surface
// at all (contracts/http.md).

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

	mu         sync.Mutex
	issuer     string
	endpoints  discoveryDocument
	keys       []jwk
	state      DiscoveryState
	lastHealth time.Time

	jwksURI string
}

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

type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	KTY string `json:"kty"`
	Use string `json:"use"`
	KID string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
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

// Refresh fetches discovery and signing keys. It records the outcome and returns
// the error: callers on request paths MUST treat failure as degraded, never as
// fatal, and readiness MUST NOT consult it (FR-013, ADR-0003).
func (p *Provider) Refresh(ctx context.Context) error {
	document, err := p.fetchDiscovery(ctx)
	if err != nil {
		p.mark(err)
		return err
	}
	if err := p.validateDiscovery(document); err != nil {
		p.mark(err)
		return err
	}
	if err := p.fetchKeys(ctx, document.JWKSURI); err != nil {
		p.mark(err)
		return err
	}
	p.mu.Lock()
	p.endpoints = document
	p.jwksURI = document.JWKSURI
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

// AuthorizeURL builds the provider authorization request using S256 PKCE. When
// the last provider round trip failed it makes one discovery attempt so a
// recovered provider is usable again without a background poller; a still-failing
// provider returns an error, which the handler reports as
// authentication_unavailable (FR-012, FR-013).
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
	values := url.Values{
		"response_type":         []string{"code"},
		"client_id":             []string{p.config.ClientID},
		"redirect_uri":          []string{p.config.RedirectURL},
		"state":                 []string{state},
		"nonce":                 []string{nonce},
		"code_challenge":        []string{ChallengeFromVerifier(verifier)},
		"code_challenge_method": []string{"S256"},
	}
	endpoint, err := url.Parse(endpoints.AuthorizationEndpoint)
	if err != nil {
		return "", fmt.Errorf("oidc: authorization endpoint: %w", err)
	}
	rawQuery := values.Encode()
	// url.Values.Encode writes spaces as '+', which the prompt page escapes into
	// the HTML entity form and any URL parser then reads as a fragment start. The
	// scope list is space-separated (RFC 6749), so write its separators as %20.
	if len(rawQuery) > 0 {
		rawQuery += "&"
	}
	rawQuery += "scope=" + strings.ReplaceAll(url.QueryEscape(strings.Join(scopes, " ")), "+", "%20")
	endpoint.RawQuery = rawQuery
	return endpoint.String(), nil
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
	tokenEndpoint := p.endpoints.TokenEndpoint
	p.mu.Unlock()
	if tokenEndpoint == "" {
		return nil, errors.New("oidc: provider discovery is unavailable")
	}
	form := url.Values{
		"grant_type":    []string{"authorization_code"},
		"code":          []string{code},
		"redirect_uri":  []string{p.config.RedirectURL},
		"code_verifier": []string{verifier},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.SetBasicAuth(p.config.ClientID, p.config.ClientSecret)
	response, err := p.config.HTTPClient.Do(request)
	if err != nil {
		p.mark(err)
		return nil, errors.New("oidc: token exchange request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		p.mark(errors.New("token endpoint error"))
		return nil, fmt.Errorf("oidc: token endpoint returned HTTP %d", response.StatusCode)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		p.mark(err)
		return nil, errors.New("oidc: malformed token response")
	}
	if payload.IDToken == "" {
		p.mark(errors.New("missing id_token"))
		return nil, errors.New("oidc: provider returned no ID token")
	}
	claims, err := p.VerifyIDToken(ctx, payload.IDToken, nonce)
	if err != nil {
		return nil, err
	}
	p.mark(nil)
	return &TokenSet{AccessToken: payload.AccessToken, Claims: claims}, nil
}

// Userinfo fetches the userinfo document with a bearer access token, used when
// OIDC_GROUPS_SOURCE=userinfo (FR-004).
func (p *Provider) Userinfo(ctx context.Context, accessToken string) (map[string]any, error) {
	p.mu.Lock()
	endpoint := p.endpoints.UserinfoEndpoint
	p.mu.Unlock()
	if endpoint == "" {
		return nil, errors.New("oidc: provider has no userinfo endpoint")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := p.config.HTTPClient.Do(request)
	if err != nil {
		p.mark(err)
		return nil, errors.New("oidc: userinfo request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		p.mark(errors.New("userinfo error"))
		return nil, fmt.Errorf("oidc: userinfo endpoint returned HTTP %d", response.StatusCode)
	}
	var claims map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&claims); err != nil {
		p.mark(err)
		return nil, errors.New("oidc: malformed userinfo response")
	}
	p.mark(nil)
	return claims, nil
}

// VerifyIDToken enforces every claim check that must pass before a session is
// created (FR-002). Error text names the violated rule but never echoes token
// material (FR-009).
func (p *Provider) VerifyIDToken(ctx context.Context, token, nonce string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("oidc: id token is not a three-part JWT")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("oidc: id token header is not base64url")
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, errors.New("oidc: id token header is not JSON")
	}
	if header.Algorithm != "RS256" || header.KeyID == "" {
		return nil, fmt.Errorf("oidc: unsupported id token algorithm %q", header.Algorithm)
	}
	key, err := p.signingKey(ctx, header.KeyID)
	if err != nil {
		return nil, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("oidc: id token signature is not base64url")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return nil, errors.New("oidc: id token signature is invalid")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("oidc: id token payload is not base64url")
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, errors.New("oidc: id token payload is not JSON")
	}
	if err := p.validateClaims(claims, nonce); err != nil {
		return nil, err
	}
	return claims, nil
}

// ChallengeFromVerifier derives the S256 code challenge from a verifier.
func ChallengeFromVerifier(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (p *Provider) validateClaims(claims map[string]any, nonce string) error {
	p.mu.Lock()
	issuer := p.issuer
	p.mu.Unlock()
	if claims["iss"] != issuer {
		return errors.New("oidc: id token issuer mismatch")
	}
	if !audienceContains(claims["aud"], p.config.ClientID) {
		return errors.New("oidc: id token audience mismatch")
	}
	if claims["nonce"] != nonce {
		return errors.New("oidc: id token nonce mismatch")
	}
	expires, ok := claims["exp"].(float64)
	if !ok || p.config.Now().Unix() >= int64(expires) {
		return errors.New("oidc: id token is expired or missing exp")
	}
	if issued, ok := claims["iat"].(float64); ok && int64(issued) > p.config.Now().Unix()+60 {
		return errors.New("oidc: id token issued in the future")
	}
	subject, ok := claims["sub"].(string)
	if !ok || subject == "" {
		return errors.New("oidc: id token has no subject")
	}
	return nil
}

func (p *Provider) signingKey(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	keys, err := p.keySet(ctx)
	if err != nil {
		return nil, err
	}
	if key, ok := findRSAKey(keys, keyID); ok {
		return key, nil
	}
	// Unknown kid: one refresh covers key rotation during a live session.
	if err := p.refreshKeys(ctx); err != nil {
		return nil, err
	}
	if key, ok := findRSAKey(p.cachedKeys(), keyID); ok {
		return key, nil
	}
	return nil, errors.New("oidc: no signing key matches the id token")
}

func findRSAKey(keys []jwk, keyID string) (*rsa.PublicKey, bool) {
	for _, candidate := range keys {
		if candidate.KID != keyID || candidate.KTY != "RSA" {
			continue
		}
		modulusBytes, err := base64.RawURLEncoding.DecodeString(candidate.N)
		if err != nil {
			return nil, false
		}
		exponentBytes, err := base64.RawURLEncoding.DecodeString(candidate.E)
		if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
			return nil, false
		}
		padded := make([]byte, 4)
		copy(padded[4-len(exponentBytes):], exponentBytes)
		exponent := int(binary.BigEndian.Uint32(padded))
		if exponent < 3 {
			return nil, false
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(modulusBytes), E: exponent}, true
	}
	return nil, false
}

func (p *Provider) keySet(ctx context.Context) ([]jwk, error) {
	if keys := p.cachedKeys(); len(keys) > 0 {
		return keys, nil
	}
	p.mu.Lock()
	uri := p.jwksURI
	p.mu.Unlock()
	if uri == "" {
		return nil, errors.New("oidc: discovery is not loaded")
	}
	if err := p.fetchKeys(ctx, uri); err != nil {
		return nil, err
	}
	return p.cachedKeys(), nil
}

func (p *Provider) fetchKeys(ctx context.Context, uri string) error {
	var document jwksDocument
	if err := p.getJSON(ctx, uri, "", &document); err != nil {
		return err
	}
	if len(document.Keys) == 0 {
		return errors.New("oidc: jwks document contains no keys")
	}
	p.mu.Lock()
	p.keys = document.Keys
	p.jwksURI = uri
	p.mu.Unlock()
	return nil
}

// refreshKeys re-fetches keys from the known JWKS URI without touching
// discovery, so an unknown kid recovers from provider key rotation.
func (p *Provider) refreshKeys(ctx context.Context) error {
	p.mu.Lock()
	uri := p.jwksURI
	p.mu.Unlock()
	if uri == "" {
		return errors.New("oidc: discovery is not loaded")
	}
	return p.fetchKeys(ctx, uri)
}

func (p *Provider) cachedKeys() []jwk {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]jwk(nil), p.keys...)
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

func audienceContains(value any, expected string) bool {
	switch audience := value.(type) {
	case string:
		return audience == expected
	case []any:
		for _, item := range audience {
			if item == expected {
				return true
			}
		}
	}
	return false
}
