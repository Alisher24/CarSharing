package config

import "errors"

// InternalClient is what a process that only calls the internal API is told: the address it reaches
// the API on and the token its capability is called with.
//
// It is a shape of its own rather than a subset of the server's configuration, because such a process
// opens no database: it is given no database credential, and a loader that demanded one would make
// every client of the internal surface hold a secret it has no use for.
type InternalClient struct {
	APIURL string
	Token  string
}

// SimulatorClient is what the simulator process is told. It calls the tick operation with the token
// of the simulator capability and no other.
func SimulatorClient() (InternalClient, error) {
	return internalClient(SimulatorTokenFileVariable)
}

// DemoControlClient is what the demonstration control is told. It calls the set-to-value operation
// with the token of the demonstration capability, which is not the one the simulator is given: a
// process allowed to advance the fleet is not thereby allowed to move a vehicle by hand.
func DemoControlClient() (InternalClient, error) {
	return internalClient(DemoControlTokenFileVariable)
}

func internalClient(tokenVariable string) (InternalClient, error) {
	token, err := secretFromFile(tokenVariable)
	if err != nil {
		return InternalClient{}, err
	}
	if token == "" {
		return InternalClient{}, errors.New(tokenVariable + " is required")
	}
	return InternalClient{APIURL: internalAPIURLFromEnvironment(), Token: token}, nil
}
