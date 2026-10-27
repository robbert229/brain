// Package authtest provides a simulated OpenID Connect provider so OIDC
// behavior is testable in CI without a live identity service (constitution
// principle I). It implements discovery, JWKS, authorization, token, userinfo,
// and optional logout endpoints and mirrors the claim shapes recorded for Pocket
// ID in docs/spikes/spk-002-pocket-id.md.
//
// Production code MUST NOT import this package.
package authtest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// ClientID and ClientSecret are the fixed credentials the provider accepts.
const (
	ClientID     = "braind-test"
	ClientSecret = "test-client-secret-value"
)

// Provider is a simulated OIDC provider backed by an httptest.Server.
type Provider struct {
	t testing.TB

	mu          sync.Mutex
	server      *httptest.Server
	keys        map[string]*rsa.PrivateKey
	servedKIDs  []string
	currentKID  string
	codes       map[string]issuedCode
	access      map[string]map[string]any
	claims      map[string]any
	userinfo    map[string]any
	tokenTTL    time.Duration
	failAt      map[string]bool
	logoutReady bool
	subject     string
	authorizeFn func(w http.ResponseWriter, r *http.Request)
}

type issuedCode struct {
	challenge   string
	redirectURI string
	nonce       string
	claims      map[string]any
	userinfo    map[string]any
}

// New starts a provider with one signing key and a full discovery document.
func New(t testing.TB) *Provider {
	t.Helper()
	provider := &Provider{
		t:        t,
		keys:     make(map[string]*rsa.PrivateKey),
		codes:    make(map[string]issuedCode),
		access:   make(map[string]map[string]any),
		failAt:   make(map[string]bool),
		tokenTTL: time.Hour,
	}
	provider.subject = "test-subject-1"
	provider.claims = map[string]any{
		"email":  "test-user@example.test",
		"groups": []string{"access-brain-user"},
	}
	provider.addKey("kid-1")
	server := httptest.NewServer(http.HandlerFunc(provider.serve))
	provider.server = server
	t.Cleanup(server.Close)
	return provider
}

// URL returns the provider's discovery document URL.
func (p *Provider) URL() string {
	return p.server.URL + "/.well-known/openid-configuration"
}

// Issuer returns the provider issuer.
func (p *Provider) Issuer() string {
	return p.server.URL
}

// RedirectURI is the fixed callback path the provider registers.
func (p *Provider) RedirectURI() string {
	return p.server.URL + "/oauth2/callback"
}

// AuthorizeParams are the query parameters a caller must present to authorize.
type AuthorizeParams struct {
	State           string
	Nonce           string
	CodeChallenge   string
	ChallengeMethod string
	RedirectURI     string
	Scope           string
	ClientID        string
}

// AuthorizeURL builds an authorize request for an httptest client.
func (p *Provider) AuthorizeURL(params AuthorizeParams) string {
	if params.ClientID == "" {
		params.ClientID = ClientID
	}
	if params.RedirectURI == "" {
		params.RedirectURI = p.RedirectURI()
	}
	values := url.Values{
		"response_type":         []string{"code"},
		"client_id":             []string{params.ClientID},
		"redirect_uri":          []string{params.RedirectURI},
		"scope":                 []string{params.Scope},
		"state":                 []string{params.State},
		"nonce":                 []string{params.Nonce},
		"code_challenge":        []string{params.CodeChallenge},
		"code_challenge_method": []string{params.ChallengeMethod},
	}
	return p.server.URL + "/authorize?" + values.Encode()
}

// SetClaims replaces the claims embedded in issued ID tokens.
func (p *Provider) SetClaims(claims map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.claims = make(map[string]any, len(claims))
	for key, value := range claims {
		p.claims[key] = value
	}
}

// SetSubject overrides the ID-token subject.
func (p *Provider) SetSubject(subject string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.subject = subject
}

// SetUserinfoClaims makes the userinfo endpoint authoritative for a claim set of
// its own, so tests can prove GroupsSource=userinfo wins over the ID token. When
// unset, userinfo mirrors the ID-token claims.
func (p *Provider) SetUserinfoClaims(claims map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.userinfo = make(map[string]any, len(claims))
	for key, value := range claims {
		p.userinfo[key] = value
	}
}

// SetTokenTTL controls ID-token expiry; a negative value issues expired tokens.
func (p *Provider) SetTokenTTL(ttl time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tokenTTL = ttl
}

// SetEndpointFailure makes the named endpoint ("discovery", "jwks", "token",
// "authorize", "userinfo") answer 500 so outage and error paths are testable.
func (p *Provider) SetEndpointFailure(endpoint string, failing bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failAt[endpoint] = failing
}

// SetAuthorizeResponse installs a handler that runs instead of issuing a code,
// for provider-side errors such as ?error=access_denied.
func (p *Provider) SetAuthorizeResponse(handler func(http.ResponseWriter, *http.Request)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.authorizeFn = handler
}

// RotateKeys generates and activates a new signing key while keeping the old
// public key advertised, exercising kid lookup during rotation.
func (p *Provider) RotateKeys() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.addKeyLocked("kid-" + base64.RawURLEncoding.EncodeToString(randomBytes(3)))
}

// EnableLogout advertises and serves an RP-initiated logout endpoint.
func (p *Provider) EnableLogout() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logoutReady = true
}

// ServeHTTP routes simulated OIDC requests, so tests can drive the provider with
// ordinary http.Client requests or directly with an httptest recorder.
func (p *Provider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.serve(w, r)
}

func (p *Provider) serve(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		p.handleDiscovery(w, r)
	case "/jwks":
		p.handleJWKS(w, r)
	case "/authorize":
		p.handleAuthorize(w, r)
	case "/token":
		p.handleToken(w, r)
	case "/userinfo":
		p.handleUserinfo(w, r)
	case "/logout":
		p.handleLogout(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (p *Provider) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	if p.failing("discovery") {
		http.Error(w, "discovery unavailable", http.StatusInternalServerError)
		return
	}
	p.mu.Lock()
	logout := ""
	if p.logoutReady {
		logout = p.server.URL + "/logout"
	}
	p.mu.Unlock()
	document := map[string]any{
		"issuer":                                p.server.URL,
		"authorization_endpoint":                p.server.URL + "/authorize",
		"token_endpoint":                        p.server.URL + "/token",
		"userinfo_endpoint":                     p.server.URL + "/userinfo",
		"jwks_uri":                              p.server.URL + "/jwks",
		"end_session_endpoint":                  logout,
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "email", "groups"},
		"claims_supported":                      []string{"sub", "email", "groups"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
	}
	if logout == "" {
		delete(document, "end_session_endpoint")
	}
	p.writeJSON(w, http.StatusOK, document)
}

func (p *Provider) handleJWKS(w http.ResponseWriter, r *http.Request) {
	if p.failing("jwks") {
		http.Error(w, "jwks unavailable", http.StatusInternalServerError)
		return
	}
	p.mu.Lock()
	keys := make([]map[string]string, 0, len(p.servedKIDs))
	for _, kid := range p.servedKIDs {
		key := p.keys[kid]
		keys = append(keys, map[string]string{
			"kty": "RSA",
			"use": "sig",
			"kid": kid,
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(bigEndianBytes(key.E)),
		})
	}
	p.mu.Unlock()
	p.writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (p *Provider) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	custom := p.authorizeFn
	p.mu.Unlock()
	if custom != nil {
		custom(w, r)
		return
	}
	if p.failing("authorize") {
		http.Error(w, "authorize unavailable", http.StatusInternalServerError)
		return
	}
	queries := r.URL.Query()
	if queries.Get("client_id") != ClientID {
		http.Error(w, "unknown client", http.StatusBadRequest)
		return
	}
	if queries.Get("code_challenge_method") != "S256" || queries.Get("code_challenge") == "" {
		http.Error(w, "missing S256 PKCE challenge", http.StatusBadRequest)
		return
	}
	redirect := queries.Get("redirect_uri")
	if redirect == "" {
		http.Error(w, "missing redirect_uri", http.StatusBadRequest)
		return
	}
	code := "code-" + base64.RawURLEncoding.EncodeToString(randomBytes(12))
	p.mu.Lock()
	p.codes[code] = issuedCode{
		challenge:   queries.Get("code_challenge"),
		redirectURI: redirect,
		nonce:       queries.Get("nonce"),
		claims:      p.claimsLocked(),
		userinfo:    p.userinfoLocked(),
	}
	p.mu.Unlock()
	location := redirect + "?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(queries.Get("state"))
	http.Redirect(w, r, location, http.StatusFound)
}

func (p *Provider) handleToken(w http.ResponseWriter, r *http.Request) {
	if p.failing("token") {
		http.Error(w, "token unavailable", http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		p.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	user, password, ok := r.BasicAuth()
	if !ok || user != ClientID || password != ClientSecret {
		p.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	if r.Form.Get("grant_type") != "authorization_code" {
		p.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
		return
	}
	code := r.Form.Get("code")
	verifier := r.Form.Get("code_verifier")
	p.mu.Lock()
	stored, found := p.codes[code]
	if found {
		delete(p.codes, code)
	}
	p.mu.Unlock()
	if !found || stored.challenge == "" {
		p.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	digest := sha256.Sum256([]byte(verifier))
	if base64.RawURLEncoding.EncodeToString(digest[:]) != stored.challenge {
		p.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "pkce_verifier_mismatch"})
		return
	}
	idToken := p.signIDToken(stored.claims, stored.nonce)
	accessToken := "access-" + base64.RawURLEncoding.EncodeToString(randomBytes(12))
	p.mu.Lock()
	p.access[accessToken] = stored.userinfo
	p.mu.Unlock()
	p.writeJSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"id_token":     idToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
	})
}

func (p *Provider) handleUserinfo(w http.ResponseWriter, r *http.Request) {
	if p.failing("userinfo") {
		http.Error(w, "userinfo unavailable", http.StatusInternalServerError)
		return
	}
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if bearer == "" {
		p.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_token"})
		return
	}
	p.mu.Lock()
	stored, found := p.access[bearer]
	if !found {
		stored = p.userinfoLocked()
	}
	claims := make(map[string]any, len(stored)+2)
	for name, value := range stored {
		claims[name] = value
	}
	claims["sub"] = p.subject
	claims["iss"] = p.server.URL
	p.mu.Unlock()
	p.writeJSON(w, http.StatusOK, claims)
}

func (p *Provider) handleLogout(w http.ResponseWriter, r *http.Request) {
	if p.failing("logout") {
		http.Error(w, "logout unavailable", http.StatusInternalServerError)
		return
	}
	target := r.URL.Query().Get("post_logout_redirect_uri")
	if target == "" {
		target = "/"
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// signIDToken signs claims with the current key; used directly by tests that
// need a token outside the normal exchange.
func (p *Provider) signIDToken(claims map[string]any, nonce string) string {
	now := time.Now()
	p.mu.Lock()
	ttl := p.tokenTTL
	kid := p.currentKID
	key := p.keys[kid]
	iss := p.server.URL
	subject := p.subject
	p.mu.Unlock()

	resolved := make(map[string]any, len(claims)+6)
	for name, value := range claims {
		resolved[name] = value
	}
	resolved["iss"] = iss
	if _, explicit := resolved["sub"]; !explicit {
		resolved["sub"] = subject
	}
	if _, explicit := resolved["aud"]; !explicit {
		resolved["aud"] = ClientID
	}
	resolved["iat"] = now.Unix()
	resolved["exp"] = now.Add(ttl).Unix()
	if nonce != "" {
		resolved["nonce"] = nonce
	}

	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid})
	payload, _ := json.Marshal(resolved)
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		p.t.Fatalf("sign simulated id token: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

// IDToken returns an ID token signed by the current key with the supplied claims,
// bypassing the exchange path (for direct verification tests).
func (p *Provider) IDToken(claims map[string]any) string {
	p.mu.Lock()
	merged := p.claimsLocked()
	p.mu.Unlock()
	for name, value := range claims {
		merged[name] = value
	}
	return p.signIDToken(merged, "")
}

// KeyCount reports advertised signing keys.
func (p *Provider) KeyCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.servedKIDs)
}

func (p *Provider) userinfoLocked() map[string]any {
	if p.userinfo != nil {
		merged := make(map[string]any, len(p.userinfo))
		for name, value := range p.userinfo {
			merged[name] = value
		}
		return merged
	}
	return p.claimsLocked()
}

func (p *Provider) claimsLocked() map[string]any {
	resolved := make(map[string]any, len(p.claims)+1)
	for name, value := range p.claims {
		resolved[name] = value
	}
	return resolved
}

func (p *Provider) failing(endpoint string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.failAt[endpoint]
}

func (p *Provider) writeJSON(w http.ResponseWriter, status int, document any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(document); err != nil {
		p.t.Errorf("encode simulated response: %v", err)
	}
}

// IDTokenFromUnadvertisedKey signs claims with a key that is never published in
// the JWKS, so verifiers must reject the signature. It returns the token.
func (p *Provider) IDTokenFromUnadvertisedKey(claims map[string]any) string {
	p.mu.Lock()
	kid := "kid-unadvertised-" + base64.RawURLEncoding.EncodeToString(randomBytes(3))
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		p.t.Fatalf("generate unadvertised key: %v", err)
	}
	p.keys[kid] = key
	merged := p.claimsLocked()
	for name, value := range claims {
		merged[name] = value
	}
	iss, subject := p.server.URL, p.subject
	ttl := p.tokenTTL
	p.mu.Unlock()

	now := time.Now()
	resolved := make(map[string]any, len(merged)+5)
	for name, value := range merged {
		resolved[name] = value
	}
	resolved["iss"] = iss
	resolved["sub"] = subject
	resolved["aud"] = ClientID
	resolved["iat"] = now.Unix()
	resolved["exp"] = now.Add(ttl).Unix()

	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid})
	payload, _ := json.Marshal(resolved)
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(input))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		p.t.Fatalf("sign id token: %v", err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func (p *Provider) addKey(kid string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.addKeyLocked(kid)
}

func (p *Provider) addKeyLocked(kid string) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		p.t.Fatalf("generate signing key: %v", err)
	}
	p.keys[kid] = key
	p.currentKID = kid
	for _, served := range p.servedKIDs {
		if served == kid {
			return
		}
	}
	p.servedKIDs = append(p.servedKIDs, kid)
}

func bigEndianBytes(value int) []byte {
	buffer := make([]byte, 4)
	binary.BigEndian.PutUint32(buffer, uint32(value))
	for len(buffer) > 1 && buffer[0] == 0 {
		buffer = buffer[1:]
	}
	return buffer
}

func randomBytes(size int) []byte {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return buffer
}
