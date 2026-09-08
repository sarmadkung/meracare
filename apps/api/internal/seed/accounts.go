package seed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

/*
The Supabase accounts behind the personas.

Seeded care data is only half of a persona: the other half is being able to sign
in as them. Tokens are verified against Supabase's published keys, so the API
cannot mint one and there is no way to fake this — the account has to exist.

Only the anon key is used, the same one the mobile app ships with. The service
role key would be simpler and is deliberately not used: it can do anything to
any account, and a development tool is not worth putting it on disk for.
*/

// Accounts creates and finds the sign-in accounts for the personas.
type Accounts struct {
	URL      string
	AnonKey  string
	Password string
	Client   *http.Client
}

// Ensure returns the Supabase identity for one address, creating the account if
// it is not there yet.
func (a Accounts) Ensure(ctx context.Context, email string) (uuid.UUID, error) {
	id, err := a.signUp(ctx, email)
	if err == nil {
		return id, nil
	}

	// An account that already exists is the ordinary case on every run after
	// the first, so it is not an error — it just means asking a different way.
	if !errIsAlreadyRegistered(err) {
		return uuid.Nil, err
	}

	return a.signIn(ctx, email)
}

type identity struct {
	ID   string `json:"id"`
	User *struct {
		ID string `json:"id"`
	} `json:"user"`
}

func (i identity) uuid() (uuid.UUID, error) {
	raw := i.ID
	if i.User != nil && i.User.ID != "" {
		raw = i.User.ID
	}
	if raw == "" {
		return uuid.Nil, fmt.Errorf("seed: Supabase returned no user id")
	}
	return uuid.Parse(raw)
}

func (a Accounts) signUp(ctx context.Context, email string) (uuid.UUID, error) {
	body, err := a.post(ctx, "/auth/v1/signup", map[string]string{
		"email": email, "password": a.Password,
	})
	if err != nil {
		return uuid.Nil, err
	}

	var parsed identity
	if err := json.Unmarshal(body, &parsed); err != nil {
		return uuid.Nil, fmt.Errorf("seed: sign-up response for %s: %w", email, err)
	}

	return parsed.uuid()
}

func (a Accounts) signIn(ctx context.Context, email string) (uuid.UUID, error) {
	body, err := a.post(ctx, "/auth/v1/token?grant_type=password", map[string]string{
		"email": email, "password": a.Password,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf(
			"seed: %s already exists but would not sign in — it may have been created "+
				"with a different password, or the project requires email confirmation: %w",
			email, err,
		)
	}

	var parsed identity
	if err := json.Unmarshal(body, &parsed); err != nil {
		return uuid.Nil, fmt.Errorf("seed: sign-in response for %s: %w", email, err)
	}

	return parsed.uuid()
}

func (a Accounts) post(ctx context.Context, path string, payload map[string]string) ([]byte, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, strings.TrimRight(a.URL, "/")+path, bytes.NewReader(encoded),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("apikey", a.AnonKey)
	request.Header.Set("Content-Type", "application/json")

	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("seed: reaching Supabase: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	if response.StatusCode >= 300 {
		// The body carries Supabase's own message, which is far more useful than
		// the status. It never contains the password, which is the only secret
		// in the request.
		return nil, fmt.Errorf("seed: Supabase returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	return body, nil
}

func errIsAlreadyRegistered(err error) bool {
	message := strings.ToLower(err.Error())

	return strings.Contains(message, "already registered") ||
		strings.Contains(message, "already been registered") ||
		strings.Contains(message, "user_already_exists")
}
