// Package democontrol is the client of the internal demonstration surface. It carries one set-to-value
// command to the API and reports what the API answered, so a demonstration is driven from a terminal
// through the closed API rather than by writing the tables the API owns.
package democontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ActionPath is the internal operation this client calls. It is the specification's own route.
const ActionPath = "/internal/v1/demo/actions"

// RequestTimeout bounds one call, so a command that cannot reach the API reports that rather than
// waiting for ever.
const RequestTimeout = 10 * time.Second

// maxAnswerBytes bounds what this client reads of an answer. An answer is an identifier and a moment,
// so anything larger is not one this build wrote.
const maxAnswerBytes = 1 << 20

// Request is one demonstration command as the contract receives it: the identifier it is remembered
// by, the action it names and the values that action states.
type Request map[string]any

// Client calls the internal demonstration operation.
type Client struct {
	http    *http.Client
	baseURL string
	token   string
}

// NewClient assembles the client over one address and one credential. Both are required: a client
// without them would call the operation as an anonymous request and be refused.
func NewClient(baseURL, token string) (*Client, error) {
	if baseURL == "" {
		return nil, errors.New("the demonstration control must be told the address of the API")
	}
	if token == "" {
		return nil, errors.New("the demonstration control must be given the token its capability is called with")
	}
	return &Client{
		http:    &http.Client{Timeout: RequestTimeout},
		baseURL: baseURL,
		token:   token,
	}, nil
}

// Apply carries one command and answers the body the API replied with, which is the stored answer of
// the first attempt when this identifier was already used.
func (c *Client) Apply(ctx context.Context, action Request) ([]byte, error) {
	body, err := json.Marshal(action)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+ActionPath,
		bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.token)

	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	answer, err := io.ReadAll(io.LimitReader(response.Body, maxAnswerBytes))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the action was refused with %s: %s", response.Status, refusalOf(answer))
	}
	return answer, nil
}

// ActionID is the identifier a command is remembered by. A command run without one draws a fresh
// identifier, so running it twice applies it twice; passing the identifier of an earlier run
// reproduces that run's answer instead.
func ActionID(given string) (string, error) {
	if given != "" {
		if _, err := uuid.Parse(given); err != nil {
			return "", fmt.Errorf("the action identifier is not a UUID: %w", err)
		}
		return given, nil
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// refusalOf is what an error answer says, so a failure names the code rather than the whole envelope.
func refusalOf(answer []byte) string {
	var envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(answer, &envelope); err != nil {
		return "an answer that is not the error contract"
	}
	return envelope.Code + ": " + envelope.Message
}
