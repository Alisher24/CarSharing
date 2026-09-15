// Package simulator is the process that advances the modelled fleet. It reaches the model over the
// internal HTTP API rather than through the database, so the transaction that writes a tick is the
// one the API opened: the simulator asks for a tick and is told what it changed, and it decides
// nothing about a vehicle itself.
package simulator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

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
type Client struct {
	http    *http.Client
	baseURL string
	token   string
}

// NewClient assembles the client over one address and one credential. Both are required: a client
// without them would call the operation as an anonymous request and be refused on every tick.
func NewClient(baseURL, token string) (*Client, error) {
	if baseURL == "" {
		return nil, errors.New("the simulator must be told the address of the API")
	}
	if token == "" {
		return nil, errors.New("the simulator must be given the token its capability is called with")
	}
	return &Client{
		http:    &http.Client{Timeout: RequestTimeout},
		baseURL: baseURL,
		token:   token,
	}, nil
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
	body, err := json.Marshal(TickRequest{TickID: tickID})
	if err != nil {
		return TickResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+TickPath,
		bytes.NewReader(body))
	if err != nil {
		return TickResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.token)

	response, err := c.http.Do(request)
	if err != nil {
		return TickResult{}, err
	}
	defer response.Body.Close()

	answer, err := io.ReadAll(io.LimitReader(response.Body, maxTickBodyBytes))
	if err != nil {
		return TickResult{}, err
	}
	if response.StatusCode != http.StatusOK {
		return TickResult{}, fmt.Errorf("the tick was refused with %s: %s",
			response.Status, refusalOf(answer))
	}
	var result TickResult
	if err = json.Unmarshal(answer, &result); err != nil {
		return TickResult{}, err
	}
	return result, nil
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
