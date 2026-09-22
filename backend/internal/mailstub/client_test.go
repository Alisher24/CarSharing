package mailstub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Alisher24/CarSharing/backend/internal/platform/httpheader"
)

// The credential the delivery client is assembled with, and the letter every check below sends.
const clientToken = "delivery-capability-token-of-one-installation"

func sentLetter() Request {
	return Request{
		To:      "rider@example.test",
		Subject: "Поездка завершена",
		Text:    "Итог: 12,34 сома",
	}
}

// A delivery names the invoice it is about in the key it carries, states the letter it delivers and
// answers the receipt of the stub.
func TestADeliveryNamesTheInvoiceItIsAbout(t *testing.T) {
	var (
		seen  *http.Request
		body  MessageRequest
		token string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r
		token = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"01994342-6ba7-7000-8000-000000000001",` +
			`"accepted_at":"2026-09-15T08:32:11.123456Z"}`))
	}))
	defer server.Close()

	client := mustClient(t, server.URL)
	receipt, err := client.Deliver(context.Background(), invoiced, sentLetter())
	if err != nil {
		t.Fatalf("the delivery failed: %v", err)
	}
	if key := seen.Header.Get(DeliveryKeyHeader); key != "invoice:"+invoiced+":issued" {
		t.Errorf("the delivery carries the key %q", key)
	}
	if requestID := seen.Header.Get(httpheader.RequestID); requestID == "" {
		t.Error("the delivery carries no request identifier")
	}
	if token != "Bearer "+clientToken {
		t.Errorf("the delivery carries the credential %q", token)
	}
	if body.To != "rider@example.test" || body.Subject != "Поездка завершена" ||
		body.Text != "Итог: 12,34 сома" {
		t.Errorf("the delivery carries %+v", body)
	}
	if receipt.ID != "01994342-6ba7-7000-8000-000000000001" ||
		receipt.AcceptedAt != "2026-09-15T08:32:11.123456Z" {
		t.Errorf("the receipt is %+v", receipt)
	}
}

// A repeat is answered with the receipt of the first delivery, which is a successful delivery: the
// letter is there, and the task that produced it may be confirmed.
func TestAReplayIsASuccessfulDelivery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Idempotency-Replayed", "true")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"01994342-6ba7-7000-8000-000000000001",` +
			`"accepted_at":"2026-09-15T08:32:11.123456Z"}`))
	}))
	defer server.Close()

	if _, err := mustClient(t, server.URL).Deliver(context.Background(), invoiced, sentLetter()); err != nil {
		t.Fatalf("the repeated delivery failed: %v", err)
	}
}

// Every other answer of the stub is a failure the queue keeps the task for: a key that already holds
// another letter, a credential the stub does not accept, and an outage.
func TestEveryOtherAnswerOfTheStubIsAFailure(t *testing.T) {
	for _, refused := range []struct {
		name   string
		status int
		body   string
		expect error
	}{
		{
			name:   "a key that holds another letter",
			status: http.StatusConflict,
			body:   `{"code":"DELIVERY_CONFLICT","message":"Already held"}`,
			expect: ErrKeyTaken,
		},
		{
			name:   "a credential the stub does not accept",
			status: http.StatusUnauthorized,
			body:   `{"code":"INTERNAL_AUTHENTICATION_REQUIRED","message":"Request could not be completed"}`,
			expect: ErrRefused,
		},
		{
			name:   "an outage of the stub",
			status: http.StatusServiceUnavailable,
			body:   `{"code":"SERVICE_UNAVAILABLE","message":"Service unavailable"}`,
			expect: ErrRefused,
		},
		{
			name:   "an answer that is not the receipt",
			status: http.StatusCreated,
			body:   `not a receipt`,
			expect: nil,
		},
	} {
		t.Run(refused.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(refused.status)
				_, _ = w.Write([]byte(refused.body))
			}))
			defer server.Close()

			_, err := mustClient(t, server.URL).Deliver(context.Background(), invoiced, sentLetter())
			if err == nil {
				t.Fatal("the delivery was reported as successful")
			}
			if refused.expect != nil && !errors.Is(err, refused.expect) {
				t.Errorf("the failure is %v", err)
			}
			if refused.status != http.StatusCreated && !strings.Contains(err.Error(), "DELIVERY_CONFLICT") &&
				!strings.Contains(err.Error(), "refused") && !strings.Contains(err.Error(), "letter") {
				t.Errorf("the failure does not name the refusal: %v", err)
			}
		})
	}
}

// A stub that never answers fails the delivery rather than holding it: the queue bounds one attempt,
// and this client is given less than that bound.
func TestADeliveryThatNeverArrivesFails(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		_ = connection.Close()
	}))
	defer broken.Close()

	if _, err := mustClient(t, broken.URL).Deliver(context.Background(), invoiced, sentLetter()); err == nil {
		t.Error("a broken connection was reported as a delivery")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := mustClient(t, broken.URL).Deliver(cancelled, invoiced, sentLetter()); err == nil {
		t.Error("a cancelled delivery was reported as a delivery")
	}
}

// A client that was given no address or no credential is refused where it is assembled.
func TestAnIncompleteClientIsRefused(t *testing.T) {
	if _, err := NewClient("", clientToken, http.DefaultTransport); err == nil {
		t.Error("a client without an address was accepted")
	}
	if _, err := NewClient("http://127.0.0.1:1", "", http.DefaultTransport); err == nil {
		t.Error("a client without a credential was accepted")
	}
}

// mustClient assembles the client one check delivers with.
func mustClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	client, err := NewClient(baseURL, clientToken, http.DefaultTransport)
	if err != nil {
		t.Fatalf("the client was refused: %v", err)
	}
	return client
}
