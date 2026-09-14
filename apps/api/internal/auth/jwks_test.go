package auth_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/genxcare/api/internal/auth"
)

// signingKey is a key pair plus the JWK the test server publishes for it.
type signingKey struct {
	kid     string
	method  jwt.SigningMethod
	private any
	jwk     map[string]any
}

func newECKey(t *testing.T, kid string) signingKey {
	t.Helper()

	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate EC key: %v", err)
	}

	byteLen := (private.Curve.Params().BitSize + 7) / 8
	x := make([]byte, byteLen)
	y := make([]byte, byteLen)
	private.X.FillBytes(x)
	private.Y.FillBytes(y)

	return signingKey{
		kid:     kid,
		method:  jwt.SigningMethodES256,
		private: private,
		jwk: map[string]any{
			"kid": kid,
			"kty": "EC",
			"crv": "P-256",
			"use": "sig",
			"alg": "ES256",
			"x":   base64.RawURLEncoding.EncodeToString(x),
			"y":   base64.RawURLEncoding.EncodeToString(y),
		},
	}
}

func newRSAKey(t *testing.T, kid string) signingKey {
	t.Helper()

	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	exponent := big.NewInt(int64(private.E)).Bytes()

	return signingKey{
		kid:     kid,
		method:  jwt.SigningMethodRS256,
		private: private,
		jwk: map[string]any{
			"kid": kid,
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(private.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(exponent),
		},
	}
}

func (k signingKey) sign(t *testing.T, o tokenOverrides) string {
	t.Helper()

	if o.subject == "" {
		o.subject = uuid.NewString()
	}
	if o.audience == "" {
		o.audience = testAudience
	}
	if o.issuer == "" {
		o.issuer = testIssuer
	}
	if o.expiresAt.IsZero() {
		o.expiresAt = time.Now().Add(time.Hour)
	}

	claims := jwt.MapClaims{
		"sub":          o.subject,
		"aud":          o.audience,
		"iss":          o.issuer,
		"iat":          time.Now().Add(-time.Minute).Unix(),
		"app_metadata": map[string]any{"provider": "google"},
	}
	if o.email != "" {
		claims["email"] = o.email
	}
	if !o.omitExp {
		claims["exp"] = o.expiresAt.Unix()
	}

	token := jwt.NewWithClaims(k.method, claims)
	token.Header["kid"] = k.kid

	signed, err := token.SignedString(k.private)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// jwksServer serves a mutable key set and counts requests.
type jwksServer struct {
	*httptest.Server
	keys     atomic.Value // []signingKey
	requests atomic.Int64
	status   atomic.Int64
}

func newJWKSServer(t *testing.T, keys ...signingKey) *jwksServer {
	t.Helper()

	server := &jwksServer{}
	server.keys.Store(keys)
	server.status.Store(http.StatusOK)

	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		server.requests.Add(1)

		if status := int(server.status.Load()); status != http.StatusOK {
			w.WriteHeader(status)
			return
		}

		current, _ := server.keys.Load().([]signingKey)
		document := map[string]any{"keys": []map[string]any{}}
		jwks := make([]map[string]any, 0, len(current))
		for _, key := range current {
			jwks = append(jwks, key.jwk)
		}
		document["keys"] = jwks

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(document)
	}))
	t.Cleanup(server.Close)

	return server
}

func (s *jwksServer) setKeys(keys ...signingKey) { s.keys.Store(keys) }

// unthrottled lets a test refresh the key set on every miss. Zero would select
// the production default of one minute, which is what the throttling tests
// exercise deliberately.
const unthrottled = time.Nanosecond

func newJWKSVerifier(t *testing.T, server *jwksServer, minRefresh time.Duration) *auth.JWKSVerifier {
	t.Helper()

	verifier, err := auth.NewJWKSVerifier(auth.JWKSVerifierOptions{
		JWKSURL:            server.URL,
		Audience:           testAudience,
		Issuer:             testIssuer,
		Leeway:             30 * time.Second,
		MinRefreshInterval: minRefresh,
	})
	if err != nil {
		t.Fatalf("NewJWKSVerifier: %v", err)
	}
	return verifier
}

func TestNewJWKSVerifierRequiresURLAndAudience(t *testing.T) {
	if _, err := auth.NewJWKSVerifier(auth.JWKSVerifierOptions{Audience: testAudience}); err == nil {
		t.Error("expected an error when the JWKS URL is empty")
	}
	if _, err := auth.NewJWKSVerifier(auth.JWKSVerifierOptions{JWKSURL: "https://example.test"}); err == nil {
		t.Error("expected an error when the audience is empty")
	}
}

func TestJWKSVerifyES256(t *testing.T) {
	key := newECKey(t, "key-1")
	server := newJWKSServer(t, key)
	verifier := newJWKSVerifier(t, server, unthrottled)

	subject := uuid.New()
	claims, err := verifier.Verify(context.Background(), key.sign(t, tokenOverrides{
		subject: subject.String(),
		email:   "maria@example.com",
	}))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	if claims.AuthUserID != subject {
		t.Errorf("AuthUserID = %s, want %s", claims.AuthUserID, subject)
	}
	if claims.Email != "maria@example.com" {
		t.Errorf("Email = %q, want maria@example.com", claims.Email)
	}
	if claims.Provider != "google" {
		t.Errorf("Provider = %q, want google", claims.Provider)
	}
}

func TestJWKSVerifyRS256(t *testing.T) {
	key := newRSAKey(t, "rsa-1")
	server := newJWKSServer(t, key)
	verifier := newJWKSVerifier(t, server, unthrottled)

	if _, err := verifier.Verify(context.Background(), key.sign(t, tokenOverrides{})); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestJWKSCachesKeysAcrossRequests(t *testing.T) {
	key := newECKey(t, "key-1")
	server := newJWKSServer(t, key)
	verifier := newJWKSVerifier(t, server, time.Minute)

	for range 5 {
		if _, err := verifier.Verify(context.Background(), key.sign(t, tokenOverrides{})); err != nil {
			t.Fatalf("Verify() error = %v", err)
		}
	}

	if got := server.requests.Load(); got != 1 {
		t.Errorf("JWKS fetched %d times, want 1 — keys should be cached", got)
	}
}

// A rotated key appears as a new `kid`, which must trigger exactly one refetch.
func TestJWKSRefreshesOnKeyRotation(t *testing.T) {
	oldKey := newECKey(t, "key-1")
	server := newJWKSServer(t, oldKey)
	verifier := newJWKSVerifier(t, server, unthrottled)

	if _, err := verifier.Verify(context.Background(), oldKey.sign(t, tokenOverrides{})); err != nil {
		t.Fatalf("Verify() with the original key: %v", err)
	}

	newKey := newECKey(t, "key-2")
	server.setKeys(oldKey, newKey)

	if _, err := verifier.Verify(context.Background(), newKey.sign(t, tokenOverrides{})); err != nil {
		t.Fatalf("Verify() after rotation: %v", err)
	}
	// The previous key is still published, so tokens already issued still work.
	if _, err := verifier.Verify(context.Background(), oldKey.sign(t, tokenOverrides{})); err != nil {
		t.Fatalf("Verify() with the superseded key: %v", err)
	}
}

// A stream of tokens carrying unknown key IDs must not turn the API into a
// request amplifier against Supabase.
func TestJWKSThrottlesRefreshForUnknownKeyIDs(t *testing.T) {
	key := newECKey(t, "key-1")
	server := newJWKSServer(t, key)
	verifier := newJWKSVerifier(t, server, time.Hour)

	if _, err := verifier.Verify(context.Background(), key.sign(t, tokenOverrides{})); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	forged := newECKey(t, "attacker-key")
	for range 10 {
		if _, err := verifier.Verify(context.Background(), forged.sign(t, tokenOverrides{})); err == nil {
			t.Fatal("Verify() accepted a token signed by an unpublished key")
		}
	}

	if got := server.requests.Load(); got != 1 {
		t.Errorf("JWKS fetched %d times, want 1 — refresh should be throttled", got)
	}
}

func TestJWKSRejectsBadTokens(t *testing.T) {
	key := newECKey(t, "key-1")
	other := newECKey(t, "key-1") // same kid, different private key
	server := newJWKSServer(t, key)
	verifier := newJWKSVerifier(t, server, time.Hour)

	cases := map[string]string{
		"empty":           "",
		"garbage":         "not-a-jwt",
		"wrong key":       other.sign(t, tokenOverrides{}),
		"expired":         key.sign(t, tokenOverrides{expiresAt: time.Now().Add(-time.Hour)}),
		"missing exp":     key.sign(t, tokenOverrides{omitExp: true}),
		"wrong audience":  key.sign(t, tokenOverrides{audience: "anon"}),
		"wrong issuer":    key.sign(t, tokenOverrides{issuer: "https://attacker.example/auth/v1"}),
		"subject not uid": key.sign(t, tokenOverrides{subject: "12345"}),
	}

	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := verifier.Verify(context.Background(), token); err == nil {
				t.Fatal("Verify() accepted an invalid token")
			} else if !errors.Is(err, auth.ErrInvalidToken) {
				t.Errorf("error %v does not wrap ErrInvalidToken", err)
			}
		})
	}
}

// Algorithm confusion: the signing key is public, so an attacker can try to
// present an HS256 token signed with the public key as the shared secret.
func TestJWKSRejectsAlgorithmConfusion(t *testing.T) {
	key := newECKey(t, "key-1")
	server := newJWKSServer(t, key)
	verifier := newJWKSVerifier(t, server, unthrottled)

	private, ok := key.private.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatal("expected an ECDSA key")
	}
	publicPoint := append(private.X.Bytes(), private.Y.Bytes()...)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": uuid.NewString(),
		"aud": testAudience,
		"iss": testIssuer,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	token.Header["kid"] = key.kid

	forged, err := token.SignedString(publicPoint)
	if err != nil {
		t.Fatalf("sign HS256 token: %v", err)
	}

	if _, err := verifier.Verify(context.Background(), forged); err == nil {
		t.Fatal("Verify() accepted an HS256 token against an asymmetric key set")
	}
}

func TestJWKSRejectsTokenWithoutKid(t *testing.T) {
	key := newECKey(t, "key-1")
	server := newJWKSServer(t, key)
	verifier := newJWKSVerifier(t, server, unthrottled)

	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"sub": uuid.NewString(),
		"aud": testAudience,
		"iss": testIssuer,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString(key.private)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	if _, err := verifier.Verify(context.Background(), signed); err == nil {
		t.Fatal("Verify() accepted a token with no kid header")
	}
}

func TestJWKSVerifyFailsWhenKeySetIsUnavailable(t *testing.T) {
	key := newECKey(t, "key-1")
	server := newJWKSServer(t, key)
	server.status.Store(http.StatusInternalServerError)
	verifier := newJWKSVerifier(t, server, unthrottled)

	if _, err := verifier.Verify(context.Background(), key.sign(t, tokenOverrides{})); err == nil {
		t.Fatal("Verify() succeeded while the key set was unavailable")
	}

	// Once the key set is reachable again, verification recovers.
	server.status.Store(http.StatusOK)
	if _, err := verifier.Verify(context.Background(), key.sign(t, tokenOverrides{})); err != nil {
		t.Fatalf("Verify() did not recover: %v", err)
	}
}

func TestJWKSWarmup(t *testing.T) {
	key := newECKey(t, "key-1")
	server := newJWKSServer(t, key)
	verifier := newJWKSVerifier(t, server, time.Hour)

	if err := verifier.Warmup(context.Background()); err != nil {
		t.Fatalf("Warmup: %v", err)
	}
	if got := server.requests.Load(); got != 1 {
		t.Fatalf("JWKS fetched %d times during warmup, want 1", got)
	}

	if _, err := verifier.Verify(context.Background(), key.sign(t, tokenOverrides{})); err != nil {
		t.Fatalf("Verify() after warmup: %v", err)
	}
	if got := server.requests.Load(); got != 1 {
		t.Errorf("JWKS fetched %d times, want 1 — warmup should prime the cache", got)
	}
}

func TestJWKSWarmupReportsUnreachableKeySet(t *testing.T) {
	server := newJWKSServer(t, newECKey(t, "key-1"))
	server.status.Store(http.StatusNotFound)
	verifier := newJWKSVerifier(t, server, unthrottled)

	if err := verifier.Warmup(context.Background()); err == nil {
		t.Fatal("Warmup succeeded against a 404 key set")
	}
}

func TestSupabaseURLHelpers(t *testing.T) {
	cases := []struct {
		in         string
		wantJWKS   string
		wantIssuer string
	}{
		{
			in:         "https://ref.supabase.co",
			wantJWKS:   "https://ref.supabase.co/auth/v1/.well-known/jwks.json",
			wantIssuer: "https://ref.supabase.co/auth/v1",
		},
		{
			in:         " https://ref.supabase.co/// ",
			wantJWKS:   "https://ref.supabase.co/auth/v1/.well-known/jwks.json",
			wantIssuer: "https://ref.supabase.co/auth/v1",
		},
	}

	for _, tc := range cases {
		if got := auth.SupabaseJWKSURL(tc.in); got != tc.wantJWKS {
			t.Errorf("SupabaseJWKSURL(%q) = %q, want %q", tc.in, got, tc.wantJWKS)
		}
		if got := auth.SupabaseIssuer(tc.in); got != tc.wantIssuer {
			t.Errorf("SupabaseIssuer(%q) = %q, want %q", tc.in, got, tc.wantIssuer)
		}
	}

	if got := auth.SupabaseIssuer("  "); got != "" {
		t.Errorf("SupabaseIssuer(blank) = %q, want empty", got)
	}
}
