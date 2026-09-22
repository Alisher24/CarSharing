package config

import (
	"errors"
	"os"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
)

// The settings the mail stub process reads beyond the database it connects to. Each has one owner:
// the variable name, the value it starts at and the shape it lands in are declared here, and nothing
// outside this loader names the variable.
const (
	// MailstubInboxAddrVariable names the listener the read-only inbox is served on.
	MailstubInboxAddrVariable = "MAILSTUB_INBOX_ADDR"

	// DefaultMailstubInboxAddr is the port the contract documents for the inbox, which is the one the
	// container publishes on the host loopback address.
	DefaultMailstubInboxAddr = ":8025"

	// The files holding the credentials of the mail stub's own capabilities. The delivery token is the
	// one the worker carries; the demonstration token arms the loss of an answer and is given to no
	// process that sends a letter.
	MailstubDeliveryTokenFileVariable = "MAILSTUB_DELIVERY_TOKEN_FILE"
	MailstubDemoTokenFileVariable     = "MAILSTUB_DEMO_TOKEN_FILE"

	// MailstubCursorHMACKeyFileVariable names the file holding the key the inbox cursors are signed
	// under. It is a key of its own rather than the key of the public API: a cursor of one service
	// accepted by the other would be a cursor that binds nothing.
	MailstubCursorHMACKeyFileVariable = "MAILSTUB_CURSOR_HMAC_KEY_FILE"
)

// MailstubInboxAddrFromEnvironment reports the listener the inbox is served on, applying the default
// when the setting is absent.
func MailstubInboxAddrFromEnvironment() string {
	if value := os.Getenv(MailstubInboxAddrVariable); value != "" {
		return value
	}
	return DefaultMailstubInboxAddr
}

// MailstubServer is everything the mail stub process is told: its database, environment, two
// listeners, two capability credentials and the key its inbox cursors are signed under.
//
// It is a shape of its own rather than a subset of the API's configuration, because the process owns
// capabilities the API does not have and serves a listener the API does not publish: a loader that
// handed it the API's shape would name settings it never reads.
type MailstubServer struct {
	Database    database.Settings
	Environment Environment

	// InternalAddr is the listener the contract's internal operations are served on. It is the
	// generic HTTP_ADDR, which is also the setting the container's readiness probe reads, so the
	// probe follows the listener rather than a copy of its port.
	InternalAddr string

	// InboxAddr is the listener the read-only inbox is served on.
	InboxAddr string

	// DeliveryToken and DemoToken are the credentials of the two capabilities. Both are required: the
	// internal surface is reachable from the internal network, so a route without its credential is a
	// route anybody there could call.
	DeliveryToken string
	DemoToken     string

	// CursorSigningKey is the key the inbox cursors are issued under. An inbox that was given no key
	// refuses to start rather than signing cursors with one every installation would share.
	CursorSigningKey []byte
}

// MailstubServerFromEnvironment reads what the mail stub process is configured with. A capability
// without its secret stops the process and names the setting it is missing.
func MailstubServerFromEnvironment() (MailstubServer, error) {
	database, err := loadDatabase()
	if err != nil {
		return MailstubServer{}, err
	}
	server := MailstubServer{
		InternalAddr: HTTPAddrFromEnvironment(),
		InboxAddr:    MailstubInboxAddrFromEnvironment(),
		Database:     database,
		Environment:  loadEnvironment(),
	}
	if server.DeliveryToken, err = requiredSecret(MailstubDeliveryTokenFileVariable); err != nil {
		return MailstubServer{}, err
	}
	if server.DemoToken, err = requiredSecret(MailstubDemoTokenFileVariable); err != nil {
		return MailstubServer{}, err
	}
	key, err := secretFromFile(MailstubCursorHMACKeyFileVariable)
	if err != nil {
		return MailstubServer{}, err
	}
	if key == "" {
		return MailstubServer{}, errors.New(MailstubCursorHMACKeyFileVariable + " is required")
	}
	server.CursorSigningKey = []byte(key)
	return server, nil
}

// requiredSecret reads a secret a process cannot start without, naming the setting that was not given
// when it is absent.
func requiredSecret(variable string) (string, error) {
	secret, err := secretFromFile(variable)
	if err != nil {
		return "", err
	}
	if secret == "" {
		return "", errors.New(variable + " is required")
	}
	return secret, nil
}
