package demo

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
)

// accountDomain is the reserved domain every demonstration account lives under, so an address here
// can never collide with one a person registers.
const accountDomain = "demo.carsharing.test"

// ManualCheckAccounts are the two accounts a person signs in as to try the application by hand.
// They own no prepared rental, so whoever signs in as one starts with nothing rented.
var ManualCheckAccounts = []string{
	"demo-one@" + accountDomain,
	"demo-two@" + accountDomain,
}

// scenarioAccountAddress names the service account a prepared rental belongs to. One account per
// prepared rental, because a person and a service account alike may hold only one rental at a time.
func scenarioAccountAddress(vehicleNumber int) string {
	return fmt.Sprintf("scenario-%02d@%s", vehicleNumber, accountDomain)
}

// unreachablePasswordBytes is the entropy of the password a service account is given.
const unreachablePasswordBytes = 32

// unreachablePassword draws a password nobody is told and nobody keeps. A service account exists
// to own a prepared rental, never to be signed in as, so the value is discarded as soon as it has
// been hashed rather than written down anywhere.
func unreachablePassword() (string, error) {
	value := make([]byte, unreachablePasswordBytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// account is one demonstration account and the hash that stands for its password.
type account struct {
	email        auth.Email
	passwordHash string
}

// accountsFor hashes every demonstration account this scenario needs: one service account per
// prepared rental and the two accounts a person checks the application with. Hashing is memory-hard
// and happens here, before any transaction is opened, so a seed run does not hold one open for the
// length of a dozen hashes.
func accountsFor(vehicles []Vehicle, hasher *auth.PasswordHasher, manualPassword string) ([]account, error) {
	var accounts []account
	for _, vehicle := range vehicles {
		if vehicle.ScenarioAccount == "" {
			continue
		}
		password, err := unreachablePassword()
		if err != nil {
			return nil, err
		}
		hashed, err := hashedAccount(vehicle.ScenarioAccount, password, hasher)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, hashed)
	}
	for _, address := range ManualCheckAccounts {
		hashed, err := hashedAccount(address, manualPassword, hasher)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, hashed)
	}
	return accounts, nil
}

func hashedAccount(address, password string, hasher *auth.PasswordHasher) (account, error) {
	email, err := auth.ParseEmail(address)
	if err != nil {
		return account{}, fmt.Errorf("demonstration address %q is unusable: %w", address, err)
	}
	passwordHash, err := hasher.Hash(password)
	if err != nil {
		return account{}, err
	}
	return account{email: email, passwordHash: passwordHash}, nil
}
