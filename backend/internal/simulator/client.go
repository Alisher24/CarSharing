// Package simulator is the process that advances the modelled fleet. It reaches the model over the
// internal HTTP API rather than through the database, so the transaction that writes a tick is the
// one the API opened: the simulator asks for a tick and is told what it changed, and it decides
// nothing about a vehicle itself.
package simulator

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Alisher24/CarSharing/backend/internal/platform/internalclient"
	"github.com/google/uuid"
)

// TickPath is the internal operation this process calls. It is the specification's own route.
const TickPath = "/internal/v1/simulation/tick"

// maxTickBodyBytes bounds what this client reads of a tick answer. A tick answers with identifiers,
// so an answer larger than this is not one this build wrote.
const maxTickBodyBytes = 1 << 20

// TickRequest is what one call of the tick operation carries: the identifier the call is recognised
// by, so a repeat after a lost answer reproduces the first one rather than moving the fleet twice.
type TickRequest struct {
	TickID string `json:"tick_id"`
}

// TickResult is what one tick changed.
type TickResult struct {
	TickID           string   `json:"tick_id"`
	ServerTime       string   `json:"server_time"`
	ProcessedAt      string   `json:"processed_at"`
	ChangedVehicles  []string `json:"changed_vehicle_ids"`
	CompletedRentals []string `json:"completed_rental_ids"`
}

// Client calls the internal tick operation.
type Client struct{ call *internalclient.Client }

// NewClient assembles the client over one address and one credential. Both are required: a client
// without them would call the operation as an anonymous request and be refused on every tick.
func NewClient(baseURL, token string, transport http.RoundTripper) (*Client, error) {
	call, err := internalclient.New(baseURL, token, internalclient.Settings{Transport: transport,
		Timeout:        RequestTimeout,
		MaxAnswerBytes: maxTickBodyBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("the simulator: %w", err)
	}
	return &Client{call: call}, nil
}

// Tick advances the fleet once and answers what it changed. The identifier is drawn here, so a call
// that is repeated after a lost answer is recognised by the API as the same call.
func (c *Client) Tick(ctx context.Context) (TickResult, error) {
	tickID, err := uuid.NewRandom()
	if err != nil {
		return TickResult{}, err
	}
	return c.TickWith(ctx, tickID.String())
}

// TickWith advances the fleet under one identifier, which is what makes a repeat reproduce the first
// answer rather than advancing anything a second time.
func (c *Client) TickWith(ctx context.Context, tickID string) (TickResult, error) {
	answer, err := c.call.Post(ctx, internalclient.Call{
		Path: TickPath,
		Body: TickRequest{TickID: tickID},
	})
	if err != nil {
		return TickResult{}, err
	}
	if answer.Status != http.StatusOK {
		return TickResult{}, fmt.Errorf("the tick was refused with %d: %s", answer.Status, answer.Refusal())
	}
	var result TickResult
	if err = answer.Decode(&result); err != nil {
		return TickResult{}, err
	}
	return result, nil
}
