package idempotency

import "github.com/google/uuid"

// Owner is who a remembered command belongs to: the account that sent it, or the installation itself
// for the commands of the internal surface, which belong to no account. A command key is unique
// within its owner, so a demonstration token and an account cannot spend each other's keys.
type Owner struct {
	name   string
	sender uuid.UUID
}

// ForAccount is the owner of a command one account sent.
func ForAccount(id uuid.UUID) Owner { return Owner{name: "account:" + id.String(), sender: id} }

// ForInstallation is the owner of a command the installation sent on its own behalf: a simulation tick
// or a demonstration action. It names no account, and the stored row therefore carries none.
func ForInstallation() Owner { return Owner{name: "internal"} }

// storedSender is the account the stored row names, or nothing when the command belongs to the
// installation.
func (o Owner) storedSender() any {
	if o.sender == uuid.Nil {
		return nil
	}
	return o.sender
}
