package mailstub

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/httpheader"
	"github.com/Alisher24/CarSharing/backend/internal/platform/internalclient"
	"github.com/google/uuid"
)

// DeliverPath is the internal operation this client calls. It is the mail stub's own route.
const DeliverPath = "/internal/v1/messages"

// The bounds one delivery is given. The call is one step of a delivery the queue already bounds, so it
// is given less than that bound: a stub that does not answer in time fails this attempt rather than
// holding the task until its lease runs out.
const (
	RequestTimeout = 4 * time.Second

	// maxAnswerBytes bounds what this client reads of a receipt. A receipt is an identifier and a
	// moment, so an answer larger than this is not one this build wrote.
	maxAnswerBytes = 1 << 16
)

// The failures a delivery can meet beyond the ones the transport reports. Each names the code the stub
// answered with, because a delivery that keeps being refused has to be visible in the queue as the
// refusal it is rather than as a letter nobody sent.
var (
	// ErrRefused reports a delivery the stub answered with a status this build does not accept.
	ErrRefused = errors.New("the mail stub refused the delivery")

	// ErrKeyTaken reports a refusal a retry cannot repair: the key the letter was delivered under
	// already holds another letter. It is a defect of whatever built the letter rather than a state
	// the next attempt changes.
	ErrKeyTaken = fmt.Errorf("%w: the key already holds another letter", ErrRefused)
)

// DeliveryReceipt is what the stub answers a delivery with: the letter it holds under the key, and the
// moment it accepted it. A repeat answers the receipt of the first delivery, so a caller that sees one
// is told the truth about a letter that was already there.
type DeliveryReceipt struct {
	ID         string `json:"id"`
	AcceptedAt string `json:"accepted_at"`
}

// Client delivers letters to the mail stub.
type Client struct{ call *internalclient.Client }

// NewClient assembles the client over one address and one credential. Both are required: a client
// without them would deliver nothing and report every attempt as a failure of the request.
func NewClient(baseURL, token string, transport http.RoundTripper) (*Client, error) {
	call, err := internalclient.New(baseURL, token, internalclient.Settings{Transport: transport,
		Timeout:        RequestTimeout,
		MaxAnswerBytes: maxAnswerBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("the mail delivery: %w", err)
	}
	return &Client{call: call}, nil
}

// Deliver stores one letter under the key of the invoice it is about. The key is built here from the
// invoice the letter was rendered from, which is what keeps the two describing one invoice: a caller
// cannot hand this client a key that names another one.
//
// A delivery is successful only when the stub answered that it stored the letter — with a receipt of
// the first delivery or with the receipt a repeat is answered by. Every other answer is an error, and
// the task that produced it stays in the queue.
func (c *Client) Deliver(ctx context.Context, invoiceID string, request Request) (DeliveryReceipt, error) {
	key, err := InvoiceKey(invoiceID)
	if err != nil {
		return DeliveryReceipt{}, err
	}
	answer, err := c.call.Post(ctx, internalclient.Call{
		Path:    DeliverPath,
		Body:    MessageRequest{To: request.To, Subject: request.Subject, Text: request.Text},
		Headers: map[string]string{DeliveryKeyHeader: key.String(), httpheader.RequestID: uuid.NewString()},
	})
	if err != nil {
		return DeliveryReceipt{}, err
	}
	switch answer.Status {
	case http.StatusCreated:
		var receipt DeliveryReceipt
		if err = answer.Decode(&receipt); err != nil {
			return DeliveryReceipt{}, err
		}
		return receipt, nil
	case http.StatusConflict:
		return DeliveryReceipt{}, fmt.Errorf("%w: %s", ErrKeyTaken, answer.Refusal())
	default:
		return DeliveryReceipt{}, fmt.Errorf("%w with %d: %s", ErrRefused, answer.Status, answer.Refusal())
	}
}

// MessageRequest is the letter as the delivery operation receives it: the three fields the contract
// declares. It is a shape of this client rather than of the stub's module, because what one process
// sends is what the other reads off the wire.
type MessageRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

// The two headers the delivery operation declares. They are named here because this client is what
// fills them in on the way to the stub, and the boundary of the process that serves the operation
// reads the same names: the two are held together by a check in the package that owns the boundary,
// so a rename on one side cannot leave the other sending a header nobody reads.
const (
	DeliveryKeyHeader = "Delivery-Key"
)
