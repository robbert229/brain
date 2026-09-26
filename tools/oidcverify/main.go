// oidcverify performs a one-shot, privacy-preserving verification of an OIDC
// provider. It is a development spike tool, not braind's production OIDC
// implementation.
package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultIssuer = "https://oidc.lab.johnrowley.co"
	defaultListen = "127.0.0.1:18765"
)

type configuration struct {
	issuer       string
	clientID     string
	listen       string
	timeout      time.Duration
	reportPath   string
	verifyLogout bool
	expected     []string
}

type discoveryDocument struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	UserinfoEndpoint                  string   `json:"userinfo_endpoint"`
	JWKSURI                           string   `json:"jwks_uri"`
	EndSessionEndpoint                string   `json:"end_session_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ClaimsSupported                   []string `json:"claims_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
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

type report struct {
	ObservedAt string             `json:"observed_at"`
	Issuer     string             `json:"issuer"`
	Discovery  discoveryEvidence  `json:"discovery"`
	Flow       flowEvidence       `json:"flow"`
	IDToken    claimEvidence      `json:"id_token"`
	Userinfo   claimEvidence      `json:"userinfo"`
	Comparison comparisonEvidence `json:"comparison"`
}

type discoveryEvidence struct {
	AuthorizationCode bool `json:"authorization_code"`
	PKCES256          bool `json:"pkce_s256"`
	PublicClient      bool `json:"public_client_authentication"`
	GroupsScope       bool `json:"groups_scope"`
	GroupsClaim       bool `json:"groups_claim"`
	EmailClaim        bool `json:"email_claim"`
	UserinfoEndpoint  bool `json:"userinfo_endpoint"`
	EndSession        bool `json:"end_session_endpoint"`
}

type flowEvidence struct {
	AuthorizationCodeReceived bool `json:"authorization_code_received"`
	PKCES256Used              bool `json:"pkce_s256_used"`
	TokenExchangeSucceeded    bool `json:"token_exchange_succeeded"`
	IDTokenSignatureValidated bool `json:"id_token_signature_validated"`
	IDTokenClaimsValidated    bool `json:"id_token_claims_validated"`
	UserinfoFetched           bool `json:"userinfo_fetched"`
	LogoutRequested           bool `json:"logout_requested"`
	LogoutCallbackObserved    bool `json:"logout_callback_observed"`
}

type claimEvidence struct {
	SubjectPresent       bool            `json:"subject_present"`
	EmailPresent         bool            `json:"email_present"`
	EmailIsString        bool            `json:"email_is_string"`
	EmailVerifiedPresent bool            `json:"email_verified_present"`
	GroupsPresent        bool            `json:"groups_present"`
	GroupsAreStrings     bool            `json:"groups_are_array_of_strings"`
	GroupCount           int             `json:"group_count"`
	ExpectedMembership   map[string]bool `json:"expected_group_membership"`
}

type comparisonEvidence struct {
	GroupSetsEqual bool `json:"id_token_and_userinfo_groups_equal"`
}

type verificationResult struct {
	report report
	err    error
}

func main() {
	cfg := parseFlags()
	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "oidc verification failed: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() configuration {
	var expected string
	cfg := configuration{}
	flag.StringVar(&cfg.issuer, "issuer", defaultIssuer, "exact OIDC issuer URL")
	flag.StringVar(&cfg.clientID, "client-id", "", "disposable public-client ID")
	flag.StringVar(&cfg.listen, "listen", defaultListen, "loopback callback listen address")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "authorization timeout")
	flag.StringVar(&cfg.reportPath, "report", "", "optional sanitized JSON report path")
	flag.BoolVar(&cfg.verifyLogout, "verify-logout", false, "request RP-initiated logout and wait for its callback")
	flag.StringVar(&expected, "expected-groups", "access-brain-admin,access-brain-user,access-brain-viewer", "comma-separated group names reported only as membership booleans")
	flag.Parse()

	for _, value := range strings.Split(expected, ",") {
		value = strings.TrimSpace(value)
		if value != "" {
			cfg.expected = append(cfg.expected, value)
		}
	}
	return cfg
}

func run(parent context.Context, cfg configuration, output io.Writer) error {
	if cfg.clientID == "" {
		return errors.New("-client-id is required")
	}
	issuer, err := normalizeIssuer(cfg.issuer)
	if err != nil {
		return err
	}
	cfg.issuer = issuer
	if cfg.timeout <= 0 {
		return errors.New("-timeout must be positive")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	discovery, err := fetchJSON[discoveryDocument](parent, client, issuer+"/.well-known/openid-configuration", "")
	if err != nil {
		return fmt.Errorf("fetch discovery: %w", err)
	}
	if discovery.Issuer != issuer {
		return fmt.Errorf("discovery issuer mismatch: got %q", discovery.Issuer)
	}
	if err := validateEndpointOrigins(issuer, discovery); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", cfg.listen)
	if err != nil {
		return fmt.Errorf("listen for callback: %w", err)
	}
	defer listener.Close()
	callbackBase := "http://" + listener.Addr().String()
	redirectURI := callbackBase + "/callback"
	logoutRedirectURI := callbackBase + "/logout-complete"

	state, err := randomURLToken(32)
	if err != nil {
		return err
	}
	nonce, err := randomURLToken(32)
	if err != nil {
		return err
	}
	verifier, err := randomURLToken(48)
	if err != nil {
		return err
	}
	challengeDigest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeDigest[:])

	rep := report{
		ObservedAt: time.Now().UTC().Format(time.RFC3339),
		Issuer:     issuer,
		Discovery: discoveryEvidence{
			AuthorizationCode: contains(discovery.ResponseTypesSupported, "code"),
			PKCES256:          contains(discovery.CodeChallengeMethodsSupported, "S256"),
			PublicClient:      contains(discovery.TokenEndpointAuthMethodsSupported, "none"),
			GroupsScope:       contains(discovery.ScopesSupported, "groups"),
			GroupsClaim:       contains(discovery.ClaimsSupported, "groups"),
			EmailClaim:        contains(discovery.ClaimsSupported, "email"),
			UserinfoEndpoint:  discovery.UserinfoEndpoint != "",
			EndSession:        discovery.EndSessionEndpoint != "",
		},
	}

	resultChannel := make(chan verificationResult, 1)
	var resultMutex sync.Mutex
	var completed bool
	var logoutCompleted bool
	var pendingReport report

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(response http.ResponseWriter, request *http.Request) {
		resultMutex.Lock()
		if completed {
			resultMutex.Unlock()
			http.Error(response, "verification callback already used", http.StatusConflict)
			return
		}
		completed = true
		resultMutex.Unlock()
		if request.URL.Query().Get("state") != state {
			resultChannel <- verificationResult{err: errors.New("callback state mismatch")}
			http.Error(response, "OIDC state mismatch", http.StatusBadRequest)
			return
		}
		if providerError := request.URL.Query().Get("error"); providerError != "" {
			resultChannel <- verificationResult{err: fmt.Errorf("provider returned %s", providerError)}
			http.Error(response, "OIDC provider returned an error", http.StatusBadRequest)
			return
		}
		code := request.URL.Query().Get("code")
		if code == "" {
			resultChannel <- verificationResult{err: errors.New("callback omitted authorization code")}
			http.Error(response, "Missing authorization code", http.StatusBadRequest)
			return
		}

		verified, idToken, verifyErr := completeVerification(request.Context(), client, cfg, discovery, redirectURI, code, verifier, nonce, rep)
		if verifyErr != nil {
			resultChannel <- verificationResult{err: verifyErr}
			http.Error(response, "OIDC verification failed; see the local verifier output", http.StatusBadGateway)
			return
		}
		if cfg.verifyLogout {
			if discovery.EndSessionEndpoint == "" {
				resultChannel <- verificationResult{err: errors.New("logout verification requested but discovery has no end_session_endpoint")}
				http.Error(response, "Provider has no end-session endpoint", http.StatusBadGateway)
				return
			}
			verified.Flow.LogoutRequested = true
			resultMutex.Lock()
			pendingReport = verified
			resultMutex.Unlock()
			logoutURL, logoutErr := url.Parse(discovery.EndSessionEndpoint)
			if logoutErr != nil {
				resultChannel <- verificationResult{err: fmt.Errorf("parse end-session endpoint: %w", logoutErr)}
				http.Error(response, "Invalid end-session endpoint", http.StatusBadGateway)
				return
			}
			query := logoutURL.Query()
			query.Set("id_token_hint", idToken)
			query.Set("post_logout_redirect_uri", logoutRedirectURI)
			logoutURL.RawQuery = query.Encode()
			http.Redirect(response, request, logoutURL.String(), http.StatusFound)
			return
		}

		resultChannel <- verificationResult{report: verified}
		response.Header().Set("Content-Type", "text/plain; charset=utf-8")
		response.Header().Set("Cache-Control", "no-store")
		fmt.Fprintln(response, "Pocket ID claim verification succeeded. You may close this tab.")
	})
	mux.HandleFunc("/logout-complete", func(response http.ResponseWriter, _ *http.Request) {
		resultMutex.Lock()
		if !cfg.verifyLogout || !pendingReport.Flow.LogoutRequested || logoutCompleted {
			resultMutex.Unlock()
			http.Error(response, "Unexpected logout callback", http.StatusBadRequest)
			return
		}
		logoutCompleted = true
		completedReport := pendingReport
		resultMutex.Unlock()
		completedReport.Flow.LogoutCallbackObserved = true
		resultChannel <- verificationResult{report: completedReport}
		response.Header().Set("Content-Type", "text/plain; charset=utf-8")
		response.Header().Set("Cache-Control", "no-store")
		fmt.Fprintln(response, "Pocket ID claim and logout verification succeeded. You may close this tab.")
	})

	server := &http.Server{Handler: mux, ErrorLog: log.New(io.Discard, "", 0)}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			select {
			case resultChannel <- verificationResult{err: fmt.Errorf("callback server: %w", serveErr)}:
			default:
			}
		}
	}()
	defer server.Shutdown(context.Background())

	authorizeURL, err := url.Parse(discovery.AuthorizationEndpoint)
	if err != nil {
		return fmt.Errorf("parse authorization endpoint: %w", err)
	}
	query := authorizeURL.Query()
	query.Set("response_type", "code")
	query.Set("client_id", cfg.clientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("scope", "openid profile email groups")
	query.Set("state", state)
	query.Set("nonce", nonce)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("prompt", "login")
	authorizeURL.RawQuery = query.Encode()

	fmt.Fprintf(output, "Open this authorization URL in a browser:\n%s\n\n", authorizeURL.String())
	fmt.Fprintf(output, "Registered callback URL: %s\n", redirectURI)
	if cfg.verifyLogout {
		fmt.Fprintf(output, "Registered logout callback URL: %s\n", logoutRedirectURI)
	}
	fmt.Fprintln(output, "No tokens, authorization codes, email addresses, subjects, or unexpected group names will be written to the report.")

	ctx, cancel := context.WithTimeout(parent, cfg.timeout)
	defer cancel()
	select {
	case result := <-resultChannel:
		if result.err != nil {
			return result.err
		}
		encoded, err := json.MarshalIndent(result.report, "", "  ")
		if err != nil {
			return fmt.Errorf("encode sanitized report: %w", err)
		}
		encoded = append(encoded, '\n')
		if cfg.reportPath != "" {
			if err := os.WriteFile(cfg.reportPath, encoded, 0o600); err != nil {
				return fmt.Errorf("write sanitized report: %w", err)
			}
			fmt.Fprintf(output, "Sanitized report written to %s\n", cfg.reportPath)
			return nil
		}
		_, err = output.Write(encoded)
		return err
	case <-ctx.Done():
		return errors.New("timed out waiting for browser authorization")
	}
}

func completeVerification(ctx context.Context, client *http.Client, cfg configuration, discovery discoveryDocument, redirectURI, code, verifier, nonce string, initial report) (report, string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {cfg.clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, discovery.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return report{}, "", fmt.Errorf("create token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return report{}, "", fmt.Errorf("exchange authorization code: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return report{}, "", fmt.Errorf("token endpoint returned HTTP %d", response.StatusCode)
	}
	var tokens tokenResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&tokens); err != nil {
		return report{}, "", fmt.Errorf("decode token response: %w", err)
	}
	if tokens.AccessToken == "" || tokens.IDToken == "" {
		return report{}, "", errors.New("token response omitted access_token or id_token")
	}

	claims, err := validateIDToken(ctx, client, discovery.JWKSURI, tokens.IDToken, cfg.issuer, cfg.clientID, nonce, time.Now())
	if err != nil {
		return report{}, "", err
	}
	userinfo, err := fetchJSON[map[string]any](ctx, client, discovery.UserinfoEndpoint, tokens.AccessToken)
	if err != nil {
		return report{}, "", fmt.Errorf("fetch UserInfo: %w", err)
	}

	idEvidence, idGroups := summarizeClaims(claims, cfg.expected)
	userEvidence, userGroups := summarizeClaims(userinfo, cfg.expected)
	initial.Flow.AuthorizationCodeReceived = true
	initial.Flow.PKCES256Used = true
	initial.Flow.TokenExchangeSucceeded = true
	initial.Flow.IDTokenSignatureValidated = true
	initial.Flow.IDTokenClaimsValidated = true
	initial.Flow.UserinfoFetched = true
	initial.IDToken = idEvidence
	initial.Userinfo = userEvidence
	initial.Comparison.GroupSetsEqual = equalStrings(idGroups, userGroups)
	return initial, tokens.IDToken, nil
}

func validateIDToken(ctx context.Context, client *http.Client, jwksURI, token, issuer, clientID, nonce string, now time.Time) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("ID token is not a three-part JWT")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("decode ID-token header")
	}
	var header struct {
		Alg string `json:"alg"`
		KID string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, errors.New("parse ID-token header")
	}
	if header.Alg != "RS256" || header.KID == "" {
		return nil, fmt.Errorf("unsupported ID-token header alg=%q or missing kid", header.Alg)
	}
	jwks, err := fetchJSON[jwksDocument](ctx, client, jwksURI, "")
	if err != nil {
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}
	key, err := findRSAKey(jwks.Keys, header.KID)
	if err != nil {
		return nil, err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("decode ID-token signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return nil, errors.New("verify ID-token signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("decode ID-token payload")
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errors.New("parse ID-token claims")
	}
	if claims["iss"] != issuer {
		return nil, errors.New("ID-token issuer mismatch")
	}
	if !audienceContains(claims["aud"], clientID) {
		return nil, errors.New("ID-token audience mismatch")
	}
	if claims["nonce"] != nonce {
		return nil, errors.New("ID-token nonce mismatch")
	}
	expires, ok := claims["exp"].(float64)
	if !ok || now.Unix() >= int64(expires) {
		return nil, errors.New("ID token is expired or has invalid exp")
	}
	if subject, ok := claims["sub"].(string); !ok || subject == "" {
		return nil, errors.New("ID token has no string sub")
	}
	return claims, nil
}

func findRSAKey(keys []jwk, keyID string) (*rsa.PublicKey, error) {
	for _, candidate := range keys {
		if candidate.KID != keyID || candidate.KTY != "RSA" || candidate.N == "" || candidate.E == "" {
			continue
		}
		modulusBytes, err := base64.RawURLEncoding.DecodeString(candidate.N)
		if err != nil {
			return nil, errors.New("decode JWK modulus")
		}
		exponentBytes, err := base64.RawURLEncoding.DecodeString(candidate.E)
		if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
			return nil, errors.New("decode JWK exponent")
		}
		padded := make([]byte, 4)
		copy(padded[4-len(exponentBytes):], exponentBytes)
		exponent := int(binary.BigEndian.Uint32(padded))
		if exponent < 3 {
			return nil, errors.New("invalid JWK exponent")
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(modulusBytes), E: exponent}, nil
	}
	return nil, errors.New("no matching RSA signing key")
}

func summarizeClaims(claims map[string]any, expected []string) (claimEvidence, []string) {
	evidence := claimEvidence{ExpectedMembership: make(map[string]bool, len(expected))}
	_, evidence.SubjectPresent = claims["sub"]
	email, emailPresent := claims["email"]
	evidence.EmailPresent = emailPresent
	_, evidence.EmailIsString = email.(string)
	_, evidence.EmailVerifiedPresent = claims["email_verified"]
	rawGroups, groupsPresent := claims["groups"]
	evidence.GroupsPresent = groupsPresent
	groups := stringArray(rawGroups)
	if groupsPresent {
		_, evidence.GroupsAreStrings = rawGroups.([]any)
		evidence.GroupsAreStrings = evidence.GroupsAreStrings && len(groups) == len(rawGroups.([]any))
	}
	evidence.GroupCount = len(groups)
	for _, group := range expected {
		evidence.ExpectedMembership[group] = contains(groups, group)
	}
	sort.Strings(groups)
	return evidence, groups
}

func stringArray(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		result = append(result, text)
	}
	return result
}

func fetchJSON[T any](ctx context.Context, client *http.Client, endpoint, bearer string) (T, error) {
	var result T
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result, err
	}
	request.Header.Set("Accept", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := client.Do(request)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return result, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return result, err
	}
	return result, nil
}

func normalizeIssuer(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSuffix(raw, "/"))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("issuer must be an absolute HTTPS URL without query or fragment")
	}
	return parsed.String(), nil
}

func validateEndpointOrigins(issuer string, discovery discoveryDocument) error {
	issuerURL, _ := url.Parse(issuer)
	for name, endpoint := range map[string]string{
		"authorization_endpoint": discovery.AuthorizationEndpoint,
		"token_endpoint":         discovery.TokenEndpoint,
		"userinfo_endpoint":      discovery.UserinfoEndpoint,
		"jwks_uri":               discovery.JWKSURI,
	} {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Scheme != "https" || parsed.Host != issuerURL.Host {
			return fmt.Errorf("%s must be HTTPS on issuer host", name)
		}
	}
	return nil
}

func randomURLToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
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

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
