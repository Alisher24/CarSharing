// Package internalclient is the shared shape of a call to an internal HTTP surface: one JSON request
// that carries the token of one capability, bounded in time and in the size of the answer it reads,
// whose refusal is reported by the code the service stated rather than by the body it sent.
//
// It is deliberately one call and no operation: which paths exist, what they carry and what an answer
// means belongs to the process that calls them, and this package only decides how a call is made.
package internalclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Settings are the bounds one caller places on its calls. A call is a short request inside a larger
// piece of work, so a caller whose work has a deadline of its own gives a call less than that
// deadline rather than as much as it can.
type Settings struct {
	// Timeout bounds one call from the first byte sent to the last byte read.
	Timeout time.Duration

	// MaxAnswerBytes bounds what this client reads of an answer. An answer larger than this is not
	// one this program wrote, so it is refused instead of being held in memory.
	MaxAnswerBytes int64
}

// Client calls the internal operations of one service over one address and one capability token.
type Client struct {
	http           *http.Client
	baseURL        string
	token          string
	maxAnswerBytes int64
}

// New assembles a client over one address and one credential. Both are required, as are the time and
// answer-size bounds of a call: a caller that was given no address would call nothing, and a caller
// without a token would be refused on every call.
func New(baseURL, token string, settings Settings) (*Client, error) {
	if baseURL == "" {
		return nil, errors.New("the internal client must be told the address it calls")
	}
	if token == "" {
		return nil, errors.New("the internal client must be given the token its capability is called with")
	}
	if settings.Timeout <= 0 {
		return nil, errors.New("the internal client must be told how long a call may take")
	}
	if settings.MaxAnswerBytes <= 0 {
		return nil, errors.New("the internal client must be told how much of an answer it reads")
	}
	return &Client{
		http:           &http.Client{Timeout: settings.Timeout},
		baseURL:        baseURL,
		token:          token,
		maxAnswerBytes: settings.MaxAnswerBytes,
	}, nil
}

// Call is one internal operation as a caller states it: where it goes, what it carries, and the
// headers the contract declares for that operation alone.
type Call struct {
	Path    string
	Body    any
	Headers map[string]string
}

// Answer is what the service replied: the status it stated, the headers it set and the body it sent.
// The body is always read, because a refusal of an internal operation states its reason in the
// envelope rather than in the status alone.
type Answer struct {
	Status int
	Header http.Header
	Body   []byte
}

// Post carries one call and answers what the service replied. A transport failure — an unreachable
// service, a timeout, a broken connection — is returned as the error it is: a caller cannot tell a
// call that was refused from one that never arrived, and must not report one as the other.
func (c *Client) Post(ctx context.Context, call Call) (Answer, error) {
	body, err := json.Marshal(call.Body)
	if err != nil {
		return Answer{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+call.Path, bytes.NewReader(body))
	if err != nil {
		return Answer{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.token)
	for name, value := range call.Headers {
		request.Header.Set(name, value)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return Answer{}, err
	}
	defer response.Body.Close()

	answer, err := io.ReadAll(io.LimitReader(response.Body, c.maxAnswerBytes))
	if err != nil {
		return Answer{}, err
	}
	return Answer{Status: response.StatusCode, Header: response.Header, Body: answer}, nil
}

// Decode reads the body of an answer into the shape the caller asked for.
func (a Answer) Decode(target any) error {
	if err := json.Unmarshal(a.Body, target); err != nil {
		return fmt.Errorf("the answer is not the shape this operation declares: %w", err)
	}
	return nil
}

// Refusal is what a refusal says: the code the service answered with, so a failure names the reason
// rather than repeating the whole envelope.
func (a Answer) Refusal() string {
	var envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(a.Body, &envelope); err != nil {
		return "an answer that is not the error contract"
	}
	return envelope.Code + ": " + envelope.Message
}
