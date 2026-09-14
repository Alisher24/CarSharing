package config

import (
	"errors"
	"os"
)

// FinishLanding is what decides whether a ride may be ended where it stands. The name of the setting
// is declared with the values it admits and with the default it takes, so a value cannot be added
// here with nothing that reads it.
const (
	FinishLandingVariable = "FINISH_LANDING"
	DefaultFinishLanding  = EnforcedFinishLanding

	// EnforcedFinishLanding requires the confirmed position of the vehicle to be fresh and inside the
	// service area the reservation was made in, which is the rule the product states.
	EnforcedFinishLanding = "enforced"

	// TestRideLifecycleFinishLanding ends a ride wherever it stands. It exists so that the checks of
	// the ride lifecycle — the demo scenario and the suites that prepare rides in ways no person
	// could — can end a ride at all: those checks move energy remainders and deadlines with direct
	// SQL and would otherwise have to build a position inside the area for every one of them.
	//
	// Only the demo profile admits it, and the command that installs a demonstration is the only
	// thing that sets it, because relaxing the rule halves what a finish proves.
	TestRideLifecycleFinishLanding = "test_ride_lifecycle"
)

// finishLandingValue is a value of the setting together with the profiles that admit it. The
// demonstration profile is the one that may relax a rule a deployment keeps.
type finishLandingValue struct {
	name            string
	admittedProfile string
}

// finishLandingValues are every value this build admits. A value that is not here is refused by Load
// rather than passed on to be interpreted by whoever reads it next.
var finishLandingValues = []finishLandingValue{
	{name: EnforcedFinishLanding},
	{name: TestRideLifecycleFinishLanding, admittedProfile: DemoEnvironment},
}

// loadFinishLanding reads which ending rule this process applies, and refuses a value that is not one
// this build declares or one the profile it runs as does not admit.
func loadFinishLanding(profile string) (string, error) {
	raw := os.Getenv(FinishLandingVariable)
	if raw == "" {
		return DefaultFinishLanding, nil
	}
	for _, admitted := range finishLandingValues {
		if admitted.name != raw {
			continue
		}
		if admitted.admittedProfile != "" && admitted.admittedProfile != profile {
			return "", errors.New(FinishLandingVariable + " admits " + raw + " only in the " +
				admitted.admittedProfile + " profile")
		}
		return raw, nil
	}
	return "", errors.New(FinishLandingVariable + " must be " + EnforcedFinishLanding + " or " +
		TestRideLifecycleFinishLanding)
}
