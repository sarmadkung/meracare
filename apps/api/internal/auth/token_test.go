package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/meracare/api/internal/auth"
)

const (
	testSecret   = "super-secret-supabase-jwt-secret"
	testAudience = "authenticated"
	testIssuer   = "https://project.supabase.co/auth/v1"
)

type tokenOverrides struct {
	subject   string
	audience  string
	issuer    string
	expiresAt time.Time
	issuedAt  time.Time
	email     string
	method    jwt.SigningMethod
	secret    string
	omitExp   bool
	provider  string
}

func signToken(t *testing.T, o tokenOverrides) string {
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
	if o.issuedAt.IsZero() {
		o.issuedAt = time.Now().Add(-time.Minute)
	}
	if o.expiresAt.IsZero() {
		o.expiresAt = time.Now().Add(time.Hour)
	}
	if o.method == nil {
		o.method = jwt.SigningMethodHS256
	}
	if o.secret == "" {
		o.secret = testSecret
	}
	if o.provider == "" {
		o.provider = "apple"
	}

	claims := jwt.MapClaims{
		"sub":          o.subject,
		"aud":          o.audience,
		"iss":          o.issuer,
		"iat":          o.issuedAt.Unix(),
		"app_metadata": map[string]any{"provider": o.provider},
	}
	if o.email != "" {
		claims["email"] = o.email
	}
	if !o.omitExp {
		claims["exp"] = o.expiresAt.Unix()
	}

	signed, err := jwt.NewWithClaims(o.method, claims).SignedString([]byte(o.secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func newVerifier(t *testing.T) *auth.HS256Verifier {
	t.Helper()
	verifier, err := auth.NewHS256Verifier(auth.HS256VerifierOptions{
		Secret:   testSecret,
		Audience: testAudience,
		Issuer:   testIssuer,
		Leeway:   30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewHS256Verifier: %v", err)
	}
	return verifier
}

func TestNewHS256VerifierRequiresSecretAndAudience(t *testing.T) {
	if _, err := auth.NewHS256Verifier(auth.HS256VerifierOptions{Audience: testAudience}); err == nil {
		t.Error("expected an error when the secret is empty")
	}
	if _, err := auth.NewHS256Verifier(auth.HS256VerifierOptions{Secret: testSecret}); err == nil {
		t.Error("expected an error when the audience is empty")
	}
}

func TestVerifyValidToken(t *testing.T) {
	verifier := newVerifier(t)
	subject := uuid.New()

	claims, err := verifier.Verify(context.Background(), signToken(t, tokenOverrides{
		subject: subject.String(),
		email:   "sara@example.com",
	}))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	if claims.AuthUserID != subject {
		t.Errorf("AuthUserID = %s, want %s", claims.AuthUserID, subject)
	}
	if claims.Email != "sara@example.com" {
		t.Errorf("Email = %q, want sara@example.com", claims.Email)
	}
	if claims.Provider != "apple" {
		t.Errorf("Provider = %q, want apple", claims.Provider)
	}
	if claims.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be populated")
	}
}

func TestVerifyRejectsBadTokens(t *testing.T) {
	verifier := newVerifier(t)

	cases := map[string]string{
		"empty":            "",
		"garbage":          "not-a-jwt",
		"wrong signature":  signToken(t, tokenOverrides{secret: "a-different-secret"}),
		"expired":          signToken(t, tokenOverrides{expiresAt: time.Now().Add(-time.Hour)}),
		"missing exp":      signToken(t, tokenOverrides{omitExp: true}),
		"wrong audience":   signToken(t, tokenOverrides{audience: "anon"}),
		"wrong issuer":     signToken(t, tokenOverrides{issuer: "https://attacker.example/auth/v1"}),
		"subject not uuid": signToken(t, tokenOverrides{subject: "12345"}),
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

// An unsigned "alg: none" token must never be accepted.
func TestVerifyRejectsUnsignedToken(t *testing.T) {
	verifier := newVerifier(t)

	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": uuid.NewString(),
		"aud": testAudience,
		"iss": testIssuer,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	raw, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none-token: %v", err)
	}

	if _, err := verifier.Verify(context.Background(), raw); err == nil {
		t.Fatal("Verify() accepted an unsigned token")
	}
}

func TestVerifyToleratesClockSkewWithinLeeway(t *testing.T) {
	verifier := newVerifier(t)

	token := signToken(t, tokenOverrides{expiresAt: time.Now().Add(-10 * time.Second)})
	if _, err := verifier.Verify(context.Background(), token); err != nil {
		t.Fatalf("Verify() rejected a token expired within the 30s leeway: %v", err)
	}
}

func TestBearerToken(t *testing.T) {
	cases := []struct {
		header    string
		wantToken string
		wantOK    bool
	}{
		{"Bearer abc.def.ghi", "abc.def.ghi", true},
		{"bearer abc.def.ghi", "abc.def.ghi", true},
		{"BEARER  abc.def.ghi ", "abc.def.ghi", true},
		{"", "", false},
		{"Bearer", "", false},
		{"Bearer ", "", false},
		{"Basic abc", "", false},
		{"abc.def.ghi", "", false},
	}

	for _, tc := range cases {
		token, ok := auth.BearerToken(tc.header)
		if ok != tc.wantOK || token != tc.wantToken {
			t.Errorf("BearerToken(%q) = (%q, %v), want (%q, %v)", tc.header, token, ok, tc.wantToken, tc.wantOK)
		}
	}
}

// The API treats every Supabase provider the same: a Google-issued token is
// verified on exactly the same path as an Apple or email one, and the provider
// is recorded rather than gated on (plans/phase10.md §15).
func TestVerifyAcceptsGoogleProvider(t *testing.T) {
	t.Parallel()

	subject := uuid.NewString()
	token := signToken(t, tokenOverrides{
		subject:  subject,
		email:    "person@example.com",
		provider: "google",
	})

	claims, err := newVerifier(t).Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify() error = %v, want nil", err)
	}
	if claims.AuthUserID.String() != subject {
		t.Errorf("AuthUserID = %q, want %q", claims.AuthUserID, subject)
	}
	if claims.Provider != "google" {
		t.Errorf("Provider = %q, want google", claims.Provider)
	}
	if claims.Email != "person@example.com" {
		t.Errorf("Email = %q, want person@example.com", claims.Email)
	}
}
