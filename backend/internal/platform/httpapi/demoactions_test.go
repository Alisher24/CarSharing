package httpapi

import (
	"encoding/json"
	"fmt"
	"testing"

	internalapi "github.com/Alisher24/CarSharing/backend/internal/contracts/internalapi"
	"github.com/Alisher24/CarSharing/backend/internal/demoaction"
	"github.com/Alisher24/CarSharing/backend/internal/democontrol"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/invoices"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
)

// The bodies below are the examples the contract publishes for the five actions the internal surface
// applies, so what is read here is the request a client really sends rather than one written to fit
// the reader.
const (
	actionIdentifier = "11111111-1111-4111-8111-111111111111"
	vehicleID        = "01994342-6ba7-7000-8000-000000000001"
	rentalID         = "01994342-6ba7-7000-8000-000000000002"
)

// A request is read into the command the module applies: the action names the kind, and every field
// the contract publishes lands in the field of the command that carries it.
func TestTheContractRequestOfEveryActionBecomesItsCommand(t *testing.T) {
	for _, request := range []struct {
		body  string
		check func(rentals.DemoCommand) error
	}{
		{
			body: `{"action":"set_telemetry_state","action_id":"` + actionIdentifier + `",
				"telemetry_state":"offline","vehicle_id":"` + vehicleID + `"}`,
			check: func(command rentals.DemoCommand) error {
				return expecting(command, demoaction.SetTelemetryState, vehicleID, false, "")
			},
		},
		{
			body: `{"action":"set_position","action_id":"` + actionIdentifier + `",
				"position":{"coordinates":[74.6,42.87],"type":"Point"},"vehicle_id":"` + vehicleID + `"}`,
			check: func(command rentals.DemoCommand) error {
				if err := expecting(command, demoaction.SetPosition, vehicleID, false, ""); err != nil {
					return err
				}
				position := fleet.Position{Longitude: 74.6, Latitude: 42.87}
				if command.Position != position {
					return fmt.Errorf("the position is %+v, want %+v", command.Position, position)
				}
				return nil
			},
		},
		{
			body: `{"action":"set_energy_remaining","action_id":"` + actionIdentifier + `",
				"remaining":"12.5","source_kind":"battery","vehicle_id":"` + vehicleID + `"}`,
			check: func(command rentals.DemoCommand) error {
				if err := expecting(command, demoaction.SetEnergyRemaining, vehicleID, false, ""); err != nil {
					return err
				}
				if command.Source != fleet.SourceBattery {
					return fmt.Errorf("the source is %q, want %q", command.Source, fleet.SourceBattery)
				}
				if remaining := command.Remaining.Decimal(); remaining != "12.5" {
					return fmt.Errorf("the reserve is %q, want 12.5", remaining)
				}
				return nil
			},
		},
		{
			body: `{"action":"mark_serviced","action_id":"` + actionIdentifier + `",
				"vehicle_id":"` + vehicleID + `"}`,
			check: func(command rentals.DemoCommand) error {
				return expecting(command, demoaction.MarkServiced, vehicleID, false, "")
			},
		},
		{
			body: `{"action":"set_next_payment_outcome","action_id":"` + actionIdentifier + `",
				"outcome":"failed","rental_id":"` + rentalID + `"}`,
			check: func(command rentals.DemoCommand) error {
				if err := expecting(command, demoaction.SetNextPaymentOutcome, "", false, rentalID); err != nil {
					return err
				}
				if command.Outcome != invoices.DemoFailed {
					return fmt.Errorf("the outcome is %q, want %q", command.Outcome, invoices.DemoFailed)
				}
				return nil
			},
		},
	} {
		var action internalapi.DemoAction
		if err := json.Unmarshal([]byte(request.body), &action); err != nil {
			t.Fatalf("the contract refused its own example: %v", err)
		}
		command, err := demoCommandOf(action)
		if err != nil {
			t.Fatalf("the request could not be read: %v", err)
		}
		if err := request.check(command); err != nil {
			t.Errorf("the request %s: %v", request.body, err)
		}
	}
}

// expecting holds one command to the action it was read from and to what it names.
func expecting(
	command rentals.DemoCommand,
	kind demoaction.Kind,
	vehicle string,
	online bool,
	rental string,
) error {
	switch {
	case command.Kind != kind:
		return fmt.Errorf("the action became the kind %q, want %q", command.Kind, kind)
	case command.ActionID != actionIdentifier:
		return fmt.Errorf("the command is remembered by %q", command.ActionID)
	case command.VehicleID != vehicle:
		return fmt.Errorf("the command names the vehicle %q, want %q", command.VehicleID, vehicle)
	case command.Online != online:
		return fmt.Errorf("the command states online %t, want %t", command.Online, online)
	case command.RentalID != rental:
		return fmt.Errorf("the command names the rental %q, want %q", command.RentalID, rental)
	}
	return nil
}

// Every action the internal surface applies travels the whole way from a terminal's own statement to a
// command the module knows: the vocabulary and the module name the same actions, so a row added to one
// without the other is a failing test rather than an action that reaches the module only to be refused
// there.
func TestEveryDemonstrationActionBecomesACommandTheModuleKnows(t *testing.T) {
	for _, action := range democontrol.Actions() {
		if action.Mail {
			continue
		}
		t.Run(action.Command(), func(t *testing.T) {
			request, err := action.Request(statedFields(action))
			if err != nil {
				t.Fatalf("the %s action could not be stated: %v", action.Kind, err)
			}
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatalf("the %s request could not be written: %v", action.Kind, err)
			}
			stated, err := democontrol.Read(body)
			if err != nil {
				t.Fatalf("the %s request could not be read: %v", action.Kind, err)
			}
			command, err := demoCommand(stated)
			if err != nil {
				t.Fatalf("the %s action could not be applied: %v", action.Kind, err)
			}
			if command.Kind != action.Kind {
				t.Fatalf("the action became the kind %q, want %q", command.Kind, action.Kind)
			}
			if err := command.Validate(); err != nil {
				t.Fatalf("the module does not know the command: %v", err)
			}
		})
	}
}

// Every field the vocabulary declares is one the surface reads, and every field the surface reads is
// one the vocabulary declares: a row stating a field nobody reads is a request that reaches the module
// half-read rather than a command that fails here.
func TestEveryDemonstrationFieldIsReadByTheSurface(t *testing.T) {
	declared := map[democontrol.Field]bool{}
	for _, action := range democontrol.Actions() {
		for _, value := range action.Values {
			declared[value.Field] = true
		}
	}
	for field := range declared {
		if _, read := demoFields[field]; !read {
			t.Errorf("the vocabulary states %q, which this surface does not read", field)
		}
	}
	for field := range demoFields {
		if !declared[field] {
			t.Errorf("this surface reads %q, which no action of the vocabulary states", field)
		}
	}
}

// statedFields states one value of every kind the vocabulary declares, so an action is read as a
// command that carries every field it names.
func statedFields(action democontrol.Action) map[democontrol.Field][]string {
	stated := make(map[democontrol.Field][]string, len(action.Values))
	for _, value := range action.Values {
		stated[value.Field] = sampleStatement(value)
	}
	return stated
}

func sampleStatement(value democontrol.Value) []string {
	switch value.Kind {
	case democontrol.Word:
		return []string{value.Words[0]}
	case democontrol.Number:
		return []string{"12.5"}
	case democontrol.Point:
		return []string{"74.6", "42.87"}
	default:
		return []string{fmt.Sprintf("%s-stated", value.Field)}
	}
}
