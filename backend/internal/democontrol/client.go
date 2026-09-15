// Package democontrol is the client of the internal demonstration surface. It carries one set-to-value
// command to the API and reports what the API answered, so a demonstration is driven from a terminal
// through the closed API rather than by writing the tables the API owns.
package democontrol

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/internalclient"
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
type Client struct{ call *internalclient.Client }

// NewClient assembles the client over one address and one credential. Both are required: a client
// without them would call the operation as an anonymous request and be refused.
func NewClient(baseURL, token string) (*Client, error) {
	call, err := internalclient.New(baseURL, token, internalclient.Settings{
		Timeout:        RequestTimeout,
		MaxAnswerBytes: maxAnswerBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("the demonstration control: %w", err)
	}
	return &Client{call: call}, nil
}

// Apply carries one command and answers the body the API replied with, which is the stored answer of
// the first attempt when this identifier was already used.
func (c *Client) Apply(ctx context.Context, action Request) ([]byte, error) {
	answer, err := c.call.Post(ctx, internalclient.Call{Path: ActionPath, Body: action})
	if err != nil {
		return nil, err
	}
	if answer.Status != http.StatusOK {
		return nil, fmt.Errorf("the action was refused with %d: %s", answer.Status, answer.Refusal())
	}
	return answer.Body, nil
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
