package notifications

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The push boundary, tested against a stand-in for the provider rather than
// against the provider. What matters is how MeraCare reads each answer: a
// rejected token has to be retired, a rate limit has to be retried, and an
// outage must not look like success (plans/phase11.md §§38, 39).

func expoServer(t *testing.T, status int, body string) (*ExpoSender, *[]byte) {
	t.Helper()

	var received []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	return NewExpoSender(ExpoSenderOptions{URL: server.URL, Timeout: 2 * time.Second}), &received
}

func message(token string) PushMessage {
	return PushMessage{
		Token: token,
		Title: "Medication reminder",
		Body:  "A dose is due for Amma at 08:00.",
		Data:  map[string]string{"type": "MEDICATION_REMINDER"},
	}
}

func TestAnAcceptedMessageIsDelivered(t *testing.T) {
	t.Parallel()

	sender, received := expoServer(t, http.StatusOK, `{"data":[{"status":"ok"}]}`)

	outcomes, err := sender.Send(context.Background(), []PushMessage{message("ExponentPushToken[abc]")})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(outcomes) != 1 || !outcomes[0].Delivered {
		t.Fatalf("outcomes = %+v, want one delivered", outcomes)
	}

	var sent []map[string]any
	if err := json.Unmarshal(*received, &sent); err != nil {
		t.Fatalf("request was not the expected JSON: %v", err)
	}
	if len(sent) != 1 || sent[0]["to"] != "ExponentPushToken[abc]" {
		t.Errorf("sent %v, want the token addressed", sent)
	}
	// A silent medication reminder is a medication reminder that does not work.
	if sent[0]["sound"] != "default" {
		t.Errorf("sound = %v, want the notification to be audible", sent[0]["sound"])
	}
}

func TestARejectedTokenIsReportedAsInvalidRatherThanRetryable(t *testing.T) {
	t.Parallel()

	sender, _ := expoServer(t, http.StatusOK, `{"data":[{
		"status":"error",
		"message":"\"ExponentPushToken[abc]\" is not a registered push notification recipient",
		"details":{"error":"DeviceNotRegistered"}
	}]}`)

	outcomes, err := sender.Send(context.Background(), []PushMessage{message("ExponentPushToken[abc]")})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if !outcomes[0].TokenInvalid {
		t.Error("DeviceNotRegistered was not recognised; the token would be retried forever")
	}
	if outcomes[0].Retryable {
		t.Error("a dead device was marked retryable")
	}
	if outcomes[0].Delivered {
		t.Error("a rejected message was reported as delivered")
	}
}

func TestARateLimitIsRetryable(t *testing.T) {
	t.Parallel()

	sender, _ := expoServer(t, http.StatusOK, `{"data":[{
		"status":"error",
		"message":"Too many messages",
		"details":{"error":"MessageRateExceeded"}
	}]}`)

	outcomes, err := sender.Send(context.Background(), []PushMessage{message("ExponentPushToken[abc]")})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !outcomes[0].Retryable || outcomes[0].TokenInvalid {
		t.Errorf("outcome = %+v, want retryable and not invalid", outcomes[0])
	}
}

func TestAProviderOutageIsRetryableForTheWholeBatch(t *testing.T) {
	t.Parallel()

	sender, _ := expoServer(t, http.StatusBadGateway, `upstream is down`)

	outcomes, err := sender.Send(context.Background(),
		[]PushMessage{message("token-a"), message("token-b")})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("got %d outcomes, want one per message", len(outcomes))
	}
	for _, outcome := range outcomes {
		if !outcome.Retryable || outcome.Delivered {
			t.Errorf("outcome = %+v, want retryable and not delivered", outcome)
		}
	}
}

func TestAnUnreachableProviderIsNotMistakenForSuccess(t *testing.T) {
	t.Parallel()

	// A URL that refuses connections, which is what a provider outage looks
	// like from here.
	sender := NewExpoSender(ExpoSenderOptions{
		URL:     "http://127.0.0.1:1/push",
		Timeout: time.Second,
	})

	outcomes, err := sender.Send(context.Background(), []PushMessage{message("token-a")})
	if err != nil {
		t.Fatalf("Send returned an error rather than an outcome: %v", err)
	}
	if outcomes[0].Delivered {
		t.Error("an unreachable provider was reported as delivery")
	}
	if !outcomes[0].Retryable {
		t.Error("an unreachable provider was not marked retryable")
	}
}

func TestFewerTicketsThanMessagesLeavesTheRestUnclaimed(t *testing.T) {
	t.Parallel()

	// Two messages, one ticket. Which one was accepted is unknowable, so the
	// second must not be marked sent.
	sender, _ := expoServer(t, http.StatusOK, `{"data":[{"status":"ok"}]}`)

	outcomes, err := sender.Send(context.Background(),
		[]PushMessage{message("token-a"), message("token-b")})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !outcomes[0].Delivered {
		t.Error("the first message should have been delivered")
	}
	if outcomes[1].Delivered || !outcomes[1].Retryable {
		t.Errorf("second outcome = %+v, want undelivered and retryable", outcomes[1])
	}
}

func TestTheAccessTokenIsSentAsCredentialsAndNothingElse(t *testing.T) {
	t.Parallel()

	var header string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"status":"ok"}]}`)
	}))
	t.Cleanup(server.Close)

	sender := NewExpoSender(ExpoSenderOptions{URL: server.URL, AccessToken: "expo-secret"})
	if _, err := sender.Send(context.Background(), []PushMessage{message("token-a")}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if header != "Bearer expo-secret" {
		t.Errorf("Authorization = %q, want the access token", header)
	}
}

func TestTheDisabledSenderRefusesEverythingPermanently(t *testing.T) {
	t.Parallel()

	outcomes, err := DisabledSender{}.Send(context.Background(),
		[]PushMessage{message("token-a"), message("token-b")})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("got %d outcomes, want one per message", len(outcomes))
	}
	for _, outcome := range outcomes {
		if outcome.Delivered || outcome.Retryable {
			t.Errorf("outcome = %+v, want undelivered and not worth retrying", outcome)
		}
		if !strings.Contains(outcome.Error, "not configured") {
			t.Errorf("error = %q, want it to say push is not configured", outcome.Error)
		}
	}
}

func TestBackoffLengthensAndThenStops(t *testing.T) {
	t.Parallel()

	// Conservative on purpose: a tight retry loop against a struggling provider
	// makes the outage worse (plans/phase11.md §38).
	if backoff(1) >= backoff(2) || backoff(2) >= backoff(3) {
		t.Errorf("backoff does not lengthen: %s, %s, %s", backoff(1), backoff(2), backoff(3))
	}
	if backoff(10) != backoff(3) {
		t.Errorf("backoff keeps growing without bound: %s", backoff(10))
	}
}
