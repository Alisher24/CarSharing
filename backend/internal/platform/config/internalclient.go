package config

import "os"

// InternalAPIURLVariable names the address the processes that call the internal API reach it on. One
// setting serves the simulator and the demonstration control, because the two only ever mean the same
// API: a second name for it would be a second thing to keep in step.
const InternalAPIURLVariable = "INTERNAL_API_URL"

// defaultInternalAPIURL is the documented local profile: the API on the loopback address of the
// machine the process runs on. A deployment names the API it calls.
const defaultInternalAPIURL = "http://127.0.0.1:8080"

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
	return internalClientOf(tokenVariable, InternalAPIURLVariable, defaultInternalAPIURL)
}

// internalClientOf reads the credential one capability is called with and the address it calls, which
// is the shape every client of an internal surface is given: a file holding a token, and a URL whose
// default is the documented local profile of the caller.
func internalClientOf(tokenVariable, urlVariable, defaultURL string) (InternalClient, error) {
	token, err := requiredSecret(tokenVariable)
	if err != nil {
		return InternalClient{}, err
	}
	apiURL := defaultURL
	if value := os.Getenv(urlVariable); value != "" {
		apiURL = value
	}
	return InternalClient{APIURL: apiURL, Token: token}, nil
}
