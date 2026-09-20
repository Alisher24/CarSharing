package internalclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testToken = "capability-token-of-one-installation"
	testPath  = "/internal/v1/something"
)

func mustClient(t *testing.T, baseURL string, settings Settings) *Client {
	t.Helper()
	client, err := New(baseURL, testToken, settings)
	if err != nil {
		t.Fatalf("the client was refused: %v", err)
	}
	return client
}

// One call carries the body it was given as JSON, the bearer credential of its capability, the media
// type the contract declares, and the headers the operation itself declares.
func TestOneCallCarriesWhatItWasGiven(t *testing.T) {
	var (
		seen  *http.Request
		body  map[string]string
		token string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r
		token = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"01994342-6ba7-7000-8000-000000000001"}`))
	}))
	defer server.Close()

	client := mustClient(t, server.URL, Settings{Timeout: time.Second, MaxAnswerBytes: 1 << 16})
	answer, err := client.Post(context.Background(), Call{
		Path:    testPath,
		Body:    map[string]string{"subject": "Письмо"},
		Headers: map[string]string{"Delivery-Key": "invoice:one:issued"},
	})
	if err != nil {
		t.Fatalf("the call failed: %v", err)
	}
	if answer.Status != http.StatusCreated {
		t.Errorf("the answer states %d", answer.Status)
	}
	if seen.URL.Path != testPath {
		t.Errorf("the call reached %s", seen.URL.Path)
	}
	if token != "Bearer "+testToken {
		t.Errorf("the call carried %q", token)
	}
	if seen.Header.Get("Content-Type") != "application/json" {
		t.Errorf("the call carried the media type %q", seen.Header.Get("Content-Type"))
	}
	if seen.Header.Get("Delivery-Key") != "invoice:one:issued" {
		t.Errorf("the call carried the header %q", seen.Header.Get("Delivery-Key"))
	}
	if body["subject"] != "Письмо" {
		t.Errorf("the call carried %v", body)
	}

	var decoded struct {
		ID string `json:"id"`
	}
	if err = answer.Decode(&decoded); err != nil {
		t.Fatalf("the answer could not be read: %v", err)
	}
	if decoded.ID != "01994342-6ba7-7000-8000-000000000001" {
		t.Errorf("the answer carries %q", decoded.ID)
	}
}

// A refusal is reported by the code the service stated rather than by the whole envelope.
func TestARefusalIsReportedByItsCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"DELIVERY_CONFLICT","message":"Already held"}`))
	}))
	defer server.Close()

	client := mustClient(t, server.URL, Settings{Timeout: time.Second, MaxAnswerBytes: 1 << 16})
	answer, err := client.Post(context.Background(), Call{Path: testPath, Body: map[string]string{}})
	if err != nil {
		t.Fatalf("the call failed: %v", err)
	}
	if answer.Status != http.StatusConflict {
		t.Errorf("the answer states %d", answer.Status)
	}
	if refusal := answer.Refusal(); refusal != "DELIVERY_CONFLICT: Already held" {
		t.Errorf("the refusal says %q", refusal)
	}
}

// A service that does not answer within the bound of the call fails it, and so does one that closes
// the connection without an answer: neither is reported as a refusal, because neither was one.
func TestACallThatNeverArrivesFails(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()
	client := mustClient(t, slow.URL, Settings{Timeout: 50 * time.Millisecond, MaxAnswerBytes: 1 << 16})
	if _, err := client.Post(context.Background(), Call{Path: testPath}); err == nil {
		t.Error("a call that timed out was reported as answered")
	}

	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		_ = connection.Close()
	}))
	defer broken.Close()
	client = mustClient(t, broken.URL, Settings{Timeout: time.Second, MaxAnswerBytes: 1 << 16})
	if _, err := client.Post(context.Background(), Call{Path: testPath}); err == nil {
		t.Error("a broken connection was reported as answered")
	}
}

// What this client reads of an answer is bounded, so a service that answers more than the operation
// declares cannot make the caller hold it.
func TestAnAnswerIsReadWithinItsBound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", 4096)))
	}))
	defer server.Close()

	client := mustClient(t, server.URL, Settings{Timeout: time.Second, MaxAnswerBytes: 64})
	answer, err := client.Post(context.Background(), Call{Path: testPath})
	if err != nil {
		t.Fatalf("the call failed: %v", err)
	}
	if len(answer.Body) != 64 {
		t.Errorf("the answer was read as %d bytes", len(answer.Body))
	}
}

// A client that was given no address, no credential or no bound on a call is refused where it is
// assembled rather than at its first call.
func TestAnIncompleteClientIsRefused(t *testing.T) {
	for _, refused := range []struct {
		name   string
		url    string
		token  string
		limits Settings
	}{
		{name: "no address", token: testToken, limits: Settings{Timeout: time.Second, MaxAnswerBytes: 1 << 16}},
		{
			name:   "no credential",
			url:    "http://127.0.0.1:1",
			limits: Settings{Timeout: time.Second, MaxAnswerBytes: 1 << 16},
		},
		{
			name:  "no timeout",
			url:   "http://127.0.0.1:1",
			token: testToken,
			limits: Settings{
				MaxAnswerBytes: 1 << 16,
			},
		},
		{
			name:  "no bound on the answer",
			url:   "http://127.0.0.1:1",
			token: testToken,
			limits: Settings{
				Timeout: time.Second,
			},
		},
	} {
		t.Run(refused.name, func(t *testing.T) {
			if _, err := New(refused.url, refused.token, refused.limits); err == nil {
				t.Error("the client was accepted")
			}
		})
	}
}
