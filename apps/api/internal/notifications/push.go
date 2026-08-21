package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// The push boundary.
//
// One interface, one implementation, and a composition root that can swap it.
// MeraCare runs on Expo, so Expo's push service is the right provider today: it
// speaks to both APNs and FCM, needs no Apple key or Firebase service account
// in this repository, and is what the existing mobile stack already produces
// tokens for. Building APNs and FCM transports directly would mean holding two
// sets of production credentials to reach the same phones
// (plans/phase11.md §22).
//
// It is behind an interface anyway, because "which push service" is exactly the
// kind of decision that changes once, late, and expensively. Nothing outside
// this file knows what Expo is.

// PushMessage is one notification addressed to one device.
type PushMessage struct {
	// Token is a credential for making somebody's phone buzz. It is never
	// logged, never returned by an endpoint, and never written into
	// notifications.last_error (plans/phase11.md §24).
	Token string
	Title string
	Body  string
	// Data is the identifiers the app needs to open the right screen. Nothing
	// medical, and nothing that grants access — the destination is authorized
	// again when it loads (plans/phase11.md §§31, 58).
	Data map[string]string
}

// PushOutcome is what happened to one message.
type PushOutcome struct {
	Token string
	// Delivered means the provider accepted the message. It does not mean the
	// phone showed it: no push service can promise that, and pretending
	// otherwise would make "sent" a claim MeraCare cannot support.
	Delivered bool
	// Retryable distinguishes "the provider was briefly unavailable" from "this
	// message will never be accepted". Only the former is worth attempting
	// again (plans/phase11.md §38).
	Retryable bool
	// TokenInvalid means the device is gone — uninstalled, or the token
	// rotated. The token is retired rather than retried (plans/phase11.md §39).
	TokenInvalid bool
	// Error is the provider's own message, kept for support. It never contains
	// the token, because the token is stripped before it gets here.
	Error string
}

// PushSender delivers messages to devices.
type PushSender interface {
	Send(ctx context.Context, messages []PushMessage) ([]PushOutcome, error)
}

// DisabledSender is the sender used when no push provider is configured.
//
// It reports every message as permanently undeliverable, which is the truth:
// there is nowhere to send it. The notification still exists and still appears
// in the inbox, so the app is fully usable with no push credentials at all —
// which is the state MeraCare is in until an EAS project is set up
// (plans/phase11.md §43).
type DisabledSender struct{}

// Send reports every message as undeliverable and not worth retrying.
func (DisabledSender) Send(_ context.Context, messages []PushMessage) ([]PushOutcome, error) {
	outcomes := make([]PushOutcome, 0, len(messages))
	for _, message := range messages {
		outcomes = append(outcomes, PushOutcome{
			Token: message.Token,
			Error: "push delivery is not configured",
		})
	}
	return outcomes, nil
}

// expoPushURL is Expo's push endpoint.
const expoPushURL = "https://exp.host/--/api/v2/push/send"

// ExpoSender delivers through Expo's push service.
type ExpoSender struct {
	client *http.Client
	url    string
	// accessToken is optional, and is a secret. It belongs in the environment
	// and nowhere else (plans/phase11.md §68).
	accessToken string
}

// ExpoSenderOptions configures the sender.
type ExpoSenderOptions struct {
	// AccessToken enables Expo's enhanced push security when the project has it
	// switched on. Empty is valid and is the common case.
	AccessToken string
	// URL overrides Expo's endpoint. Tests use it; deployments do not.
	URL string
	// Timeout bounds one request.
	Timeout time.Duration
}

// NewExpoSender builds a sender.
func NewExpoSender(opts ExpoSenderOptions) *ExpoSender {
	url := strings.TrimSpace(opts.URL)
	if url == "" {
		url = expoPushURL
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &ExpoSender{
		client:      &http.Client{Timeout: timeout},
		url:         url,
		accessToken: strings.TrimSpace(opts.AccessToken),
	}
}

// expoRequest is one message in Expo's wire format.
type expoRequest struct {
	To    string            `json:"to"`
	Title string            `json:"title"`
	Body  string            `json:"body"`
	Data  map[string]string `json:"data,omitempty"`
	// Sound is what makes a reminder audible on iOS. A silent medication
	// reminder is a medication reminder that does not work.
	Sound string `json:"sound"`
	// Priority asks Android to deliver promptly rather than batching into a
	// maintenance window, which for a dose that is due in fifteen minutes is
	// the difference between useful and pointless.
	Priority string `json:"priority"`
}

// expoResponse is Expo's reply: one ticket per message, in order.
type expoResponse struct {
	Data []struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Details struct {
			Error string `json:"error"`
		} `json:"details"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// Send delivers a batch and reports each outcome.
//
// Expo replies with one ticket per message, in the order sent. A transport
// failure is retryable for every message in the batch; a per-message error is
// judged on its own.
func (s *ExpoSender) Send(ctx context.Context, messages []PushMessage) ([]PushOutcome, error) {
	if len(messages) == 0 {
		return nil, nil
	}

	payload := make([]expoRequest, 0, len(messages))
	for _, message := range messages {
		payload = append(payload, expoRequest{
			To:       message.Token,
			Title:    message.Title,
			Body:     message.Body,
			Data:     message.Data,
			Sound:    "default",
			Priority: "high",
		})
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode push request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build push request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if s.accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+s.accessToken)
	}

	response, err := s.client.Do(request)
	if err != nil {
		// The provider could not be reached at all. Nothing was delivered and
		// everything is worth another attempt.
		return retryableBatch(messages, "push provider unreachable"), nil
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return retryableBatch(messages, "push provider response unreadable"), nil
	}

	if response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests {
		return retryableBatch(messages, fmt.Sprintf("push provider returned %d", response.StatusCode)), nil
	}
	if response.StatusCode != http.StatusOK {
		return permanentBatch(messages, fmt.Sprintf("push provider returned %d", response.StatusCode)), nil
	}

	var decoded expoResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return permanentBatch(messages, "push provider response was not understood"), nil
	}
	if len(decoded.Errors) > 0 {
		return permanentBatch(messages, decoded.Errors[0].Message), nil
	}

	outcomes := make([]PushOutcome, 0, len(messages))
	for index, message := range messages {
		if index >= len(decoded.Data) {
			// Fewer tickets than messages. Which ones were accepted is
			// unknowable, so the rest are left pending rather than marked sent.
			outcomes = append(outcomes, PushOutcome{
				Token:     message.Token,
				Retryable: true,
				Error:     "push provider returned fewer tickets than messages",
			})
			continue
		}

		ticket := decoded.Data[index]
		if ticket.Status == "ok" {
			outcomes = append(outcomes, PushOutcome{Token: message.Token, Delivered: true})
			continue
		}

		outcomes = append(outcomes, PushOutcome{
			Token: message.Token,
			// DeviceNotRegistered is Expo's name for "this install is gone".
			TokenInvalid: ticket.Details.Error == "DeviceNotRegistered",
			Retryable:    ticket.Details.Error == "MessageRateExceeded",
			Error:        ticket.Message,
		})
	}
	return outcomes, nil
}

func retryableBatch(messages []PushMessage, reason string) []PushOutcome {
	outcomes := make([]PushOutcome, 0, len(messages))
	for _, message := range messages {
		outcomes = append(outcomes, PushOutcome{Token: message.Token, Retryable: true, Error: reason})
	}
	return outcomes
}

func permanentBatch(messages []PushMessage, reason string) []PushOutcome {
	outcomes := make([]PushOutcome, 0, len(messages))
	for _, message := range messages {
		outcomes = append(outcomes, PushOutcome{Token: message.Token, Error: reason})
	}
	return outcomes
}
