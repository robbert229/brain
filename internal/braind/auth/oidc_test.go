package auth_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/johnrowl/brain/internal/braind/auth"
	"github.com/johnrowl/brain/internal/braind/auth/authtest"
)

func newProvider(t *testing.T, options ...func(*authtest.Provider)) (*authtest.Provider, *auth.Provider) {
	t.Helper()
	simulator := authtest.New(t)
	for _, option := range options {
		option(simulator)
	}
	provider, err := auth.NewProvider(auth.ProviderConfig{
		ConfigurationURL: simulator.URL(),
		ClientID:         authtest.ClientID,
		ClientSecret:     authtest.ClientSecret,
		RedirectURL:      "/oauth2/callback",
		HTTPClient:       &http.Client{},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := provider.Refresh(context.Background()); err != nil {
		t.Fatalf("discover provider: %v", err)
	}
	return simulator, provider
}

func TestProviderDiscoversAndReportsHealth(t *testing.T) {
	_, provider := newProvider(t)
	if provider.State() != auth.DiscoveryReady {
		t.Fatalf("expected ready discovery, got %s", provider.State())
	}
	if !provider.Healthy() {
		t.Fatal("healthy provider reported unhealthy")
	}
	if provider.Issuer() == "" {
		t.Fatal("issuer must be known after discovery")
	}
	if _, ok := provider.EndSessionURL(); ok {
		t.Fatal("logout endpoint must be absent unless advertised")
	}
}

func TestProviderRefreshFailureDegradesWithoutBlocking(t *testing.T) {
	simulator, provider := newProvider(t)
	simulator.SetEndpointFailure("discovery", true)
	if err := provider.Refresh(context.Background()); err == nil {
		t.Fatal("expected refresh to report the failure")
	}
	if provider.Healthy() {
		t.Fatal("failing provider must report degraded health")
	}
	// Recovery is observed on the next attempt, which is what request paths and
	// the status view rely on.
	simulator.SetEndpointFailure("discovery", false)
	if err := provider.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh after recovery: %v", err)
	}
	if !provider.Healthy() {
		t.Fatal("provider must recover to healthy")
	}
}

func TestAuthorizeURLRequiresS256(t *testing.T) {
	simulator, provider := newProvider(t)
	simulator.SetAuthorizeResponse(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("code_challenge_method") != "S256" {
			t.Errorf("provider saw method %q", r.URL.Query().Get("code_challenge_method"))
		}
		http.Redirect(w, r, "/done", http.StatusFound)
	})
	url, err := provider.AuthorizeURL("state-1", "nonce-1", "verifier-value", nil)
	if err != nil {
		t.Fatalf("authorize url: %v", err)
	}
	if !strings.Contains(url, "code_challenge_method=S256") {
		t.Fatalf("url lacks S256: %s", url)
	}
	for _, required := range []string{"state=state-1", "nonce=nonce-1", "response_type=code"} {
		if !strings.Contains(url, required) {
			t.Fatalf("url lacks %q: %s", required, url)
		}
	}
}

func TestVerifyIDTokenRejectsInvalidTokens(t *testing.T) {
	simulator, provider := newProvider(t)
	ctx := context.Background()

	t.Run("valid token verifies", func(t *testing.T) {
		token := simulator.IDToken(map[string]any{"nonce": "nonce-1"})
		claims, err := provider.VerifyIDToken(ctx, token, "nonce-1")
		if err != nil {
			t.Fatalf("valid token rejected: %v", err)
		}
		if claims["sub"] == nil {
			t.Fatal("verified claims lack subject")
		}
	})

	t.Run("nonce mismatch", func(t *testing.T) {
		token := simulator.IDToken(map[string]any{"nonce": "other"})
		if _, err := provider.VerifyIDToken(ctx, token, "expected"); err == nil {
			t.Fatal("expected nonce mismatch to fail")
		}
	})

	t.Run("audience mismatch", func(t *testing.T) {
		token := simulator.IDToken(map[string]any{"aud": "another-client", "nonce": "nonce-1"})
		if _, err := provider.VerifyIDToken(ctx, token, "nonce-1"); err == nil {
			t.Fatal("expected audience mismatch to fail")
		}
	})

	t.Run("expired token", func(t *testing.T) {
		simulator.SetTokenTTL(-time.Minute)
		defer simulator.SetTokenTTL(time.Hour)
		token := simulator.IDToken(map[string]any{"nonce": "nonce-1"})
		if _, err := provider.VerifyIDToken(ctx, token, "nonce-1"); err == nil {
			t.Fatal("expected expired token to fail")
		}
	})

	t.Run("signature from unadvertised key", func(t *testing.T) {
		token := simulator.IDTokenFromUnadvertisedKey(map[string]any{"nonce": "nonce-1"})
		if _, err := provider.VerifyIDToken(ctx, token, "nonce-1"); err == nil {
			t.Fatal("expected unverifiable signature to fail")
		}
	})

	t.Run("unsigned token", func(t *testing.T) {
		unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","kid":"kid-x"}`)) +
			"." + base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"`+simulator.Issuer()+`","aud":"`+authtest.ClientID+`","sub":"s","exp":9999999999,"nonce":"nonce-1"}`)) +
			".c2ln"
		if _, err := provider.VerifyIDToken(ctx, unsigned, "nonce-1"); err == nil {
			t.Fatal("expected unsigned token to fail")
		}
	})

	t.Run("tampered payload", func(t *testing.T) {
		token := simulator.IDToken(map[string]any{"nonce": "nonce-1"})
		parts := strings.Split(token, ".")
		var claims map[string]any
		if err := decodeJSON(parts[1], &claims); err != nil {
			t.Fatalf("decode claims: %v", err)
		}
		claims["groups"] = []string{"access-brain-admin"}
		rebuilt, err := encodeJSON(claims)
		if err != nil {
			t.Fatalf("encode claims: %v", err)
		}
		if _, err := provider.VerifyIDToken(ctx, parts[0]+"."+rebuilt+"."+parts[2], "nonce-1"); err == nil {
			t.Fatal("expected tampered token to fail verification")
		}
	})
}

func TestVerifyIDTokenRecoversFromKeyRotation(t *testing.T) {
	simulator, provider := newProvider(t)
	simulator.RotateKeys()
	token := simulator.IDToken(map[string]any{"nonce": "nonce-1"})
	if _, err := provider.VerifyIDToken(context.Background(), token, "nonce-1"); err != nil {
		t.Fatalf("rotated key must verify: %v", err)
	}
}

func TestExchangeUsesBasicAuthAndPKCE(t *testing.T) {
	simulator, provider := newProvider(t)

	nonce := "nonce-exchange"
	verifier := strings.Repeat("v", 43)
	challenge := auth.ChallengeFromVerifier(verifier)
	authorizeURL := simulator.AuthorizeURL(authtest.AuthorizeParams{State: "state-1", Nonce: nonce, CodeChallenge: challenge, ChallengeMethod: "S256"})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, authorizeURL, nil)
	// Drive the provider directly so we capture the issued code.
	simulator.ServeHTTP(recorder, request)
	location := recorder.Header().Get("Location")
	code := strings.TrimPrefix(location, simulator.RedirectURI()+"?code=")
	code = strings.Split(code, "&")[0]
	if code == location {
		t.Fatalf("authorize did not redirect: %q", location)
	}

	tokens, err := provider.Exchange(context.Background(), code, verifier, nonce)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Fatal("expected an access token")
	}
	if tokens.Claims["nonce"] != nonce {
		t.Fatalf("claims nonce = %v", tokens.Claims["nonce"])
	}

	// The same code cannot be redeemed twice.
	if _, err := provider.Exchange(context.Background(), code, verifier, nonce); err == nil {
		t.Fatal("replayed code was accepted")
	}
}

func TestExchangeRejectsWrongVerifier(t *testing.T) {
	simulator, provider := newProvider(t)
	verifier := strings.Repeat("a", 43)
	challenge := auth.ChallengeFromVerifier(verifier)
	url := simulator.AuthorizeURL(authtest.AuthorizeParams{State: "s", Nonce: "n", CodeChallenge: challenge, ChallengeMethod: "S256"})
	recorder := httptest.NewRecorder()
	simulator.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, url, nil))
	code := strings.Split(strings.TrimPrefix(recorder.Header().Get("Location"), simulator.RedirectURI()+"?code="), "&")[0]

	if _, err := provider.Exchange(context.Background(), code, "wrong-verifier", "n"); err == nil {
		t.Fatal("expected pkce failure for wrong verifier")
	}
}

func TestProviderTokenEndpointFailureReportsOutage(t *testing.T) {
	simulator, provider := newProvider(t)
	simulator.SetEndpointFailure("token", true)

	challenge := auth.ChallengeFromVerifier("verifier-value")
	url := simulator.AuthorizeURL(authtest.AuthorizeParams{State: "s", Nonce: "n", CodeChallenge: challenge, ChallengeMethod: "S256"})
	recorder := httptest.NewRecorder()
	simulator.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, url, nil))
	code := strings.Split(strings.TrimPrefix(recorder.Header().Get("Location"), simulator.RedirectURI()+"?code="), "&")[0]

	if _, err := provider.Exchange(context.Background(), code, "verifier-value", "n"); err == nil {
		t.Fatal("expected exchange failure")
	}
	if provider.Healthy() {
		t.Fatal("failed exchange must mark the provider degraded")
	}
}

func TestUserinfoClaims(t *testing.T) {
	_, provider := newProvider(t)
	claims, err := provider.Userinfo(context.Background(), "access-token")
	if err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	if claims["sub"] == nil {
		t.Fatal("userinfo claims lack subject")
	}
}

func TestProviderConfigValidation(t *testing.T) {
	if _, err := auth.NewProvider(auth.ProviderConfig{ConfigurationURL: "http://example.test/.well-known/openid-configuration", ClientID: "x", RedirectURL: "/cb"}); err == nil {
		t.Fatal("non-https issuer must be rejected")
	}
	if _, err := auth.NewProvider(auth.ProviderConfig{ConfigurationURL: "https://oidc.test/.well-known/openid-configuration", RedirectURL: "/cb"}); err == nil {
		t.Fatal("missing client id must be rejected")
	}
}

func TestDiscoveryIsNotRequiredForConstruction(t *testing.T) {
	simulator := authtest.New(t)
	simulator.SetEndpointFailure("discovery", true)
	provider, err := auth.NewProvider(auth.ProviderConfig{
		ConfigurationURL: simulator.URL(),
		ClientID:         authtest.ClientID,
		ClientSecret:     authtest.ClientSecret,
		RedirectURL:      "/oauth2/callback",
	})
	if err != nil {
		t.Fatalf("construction must not require the provider: %v", err)
	}
	if provider.Healthy() {
		t.Fatal("provider must start degraded until discovery succeeds")
	}
}

func TestSigningKeysMatchJWKSFormat(t *testing.T) {
	// The simulated provider must publish keys the verifier can consume,
	// otherwise every other test would pass against an unrealistic shape.
	simulator := authtest.New(t)
	response, err := http.Get(simulator.Issuer() + "/jwks")
	if err != nil {
		t.Fatalf("fetch jwks: %v", err)
	}
	defer response.Body.Close()
	var document struct {
		Keys []struct {
			KTY string `json:"kty"`
			KID string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		t.Fatalf("decode jwks: %v", err)
	}
	if len(document.Keys) != 1 {
		t.Fatalf("expected one signing key, got %d", len(document.Keys))
	}
	key := document.Keys[0]
	if key.KTY != "RSA" || key.KID == "" {
		t.Fatalf("unexpected jwk: %+v", key)
	}
	if _, err := base64.RawURLEncoding.DecodeString(key.N); err != nil {
		t.Fatalf("modulus is not base64url: %v", err)
	}
}

func decodeJSON(segment string, destination any) error {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, destination)
}

func encodeJSON(source any) (string, error) {
	raw, err := json.Marshal(source)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
