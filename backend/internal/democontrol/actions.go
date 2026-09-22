package democontrol

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Alisher24/CarSharing/backend/internal/demoaction"
)

// Field is one value a demonstration action states. It is spelled the way the request carries it, so
// nothing here renames a field of the contract.
type Field string

const (
	VehicleIDField      Field = "vehicle_id"
	TelemetryStateField Field = "telemetry_state"
	PositionField       Field = "position"
	SourceKindField     Field = "source_kind"
	RemainingField      Field = "remaining"
	RentalIDField       Field = "rental_id"
	OutcomeField        Field = "outcome"
)

// The two values every action carries: the action it is, and the identifier the command is remembered
// by. They are stated once here because they belong to the request rather than to any one command.
const (
	ActionField   = "action"
	ActionIDField = "action_id"
)

// Kind is how one stated value is written into a request and read back out of it.
type Kind int

const (
	// Text is a value the request carries as it was stated.
	Text Kind = iota

	// Word is one of a closed set of words, which the request carries as the word itself.
	Word

	// Number is a number the request carries as its decimal text rather than as a floating-point
	// number, which is how the contract publishes what a source is left holding.
	Number

	// Point is two numbers the request carries as one GeoJSON point.
	Point
)

// Flag is one name a terminal states a value under. What the flag states is not repeated here: a
// word's hint is the words themselves, and every other flag says what it carries.
type Flag struct {
	Name string
	Hint string
}

// Value is one value a demonstration action states: where the request carries it, how it is written
// there, and how a terminal names it.
type Value struct {
	// Field is the field of the request the value is written in.
	Field Field

	// Kind is how the value is written in that field.
	Kind Kind

	// Words are the words a Word may be, in the order a terminal offers them. They are both what a
	// statement is checked against and what a terminal says about the flag that states it.
	Words []string

	// Flags are the terminal's own names for the value, one for every number it is made of: a point
	// is stated as two of them.
	Flags []Flag

	// Needed reports a value the action cannot be applied without.
	Needed bool
}

// Action is one set-to-value command of the demonstration, declared once for both the terminal that
// states it and the surface that applies it.
type Action struct {
	// Kind is the change the action makes: the identifier the contract publishes and the word the
	// module applies.
	Kind demoaction.Kind

	// Values are what the action states, in the order a terminal names them.
	Values []Value

	// Mail reports an action the mail stub applies rather than the API.
	Mail bool

	// Terminal is the subcommand a terminal applies this action under, declared only for a command
	// the terminal and the contract spell differently.
	Terminal string
}

// Command is the subcommand a terminal applies this action under: the identifier with its underscores
// written as dashes, or the name the action declares where the two differ.
func (a Action) Command() string {
	if a.Terminal != "" {
		return a.Terminal
	}
	return strings.ReplaceAll(string(a.Kind), "_", "-")
}

// actions is every set-to-value command this build applies, declared once. A new command is a new row:
// the terminal's flags and usage line and the surface's reading of a request all follow from it.
var actions = [...]Action{
	{
		Kind: demoaction.SetTelemetryState,
		Values: []Value{
			{
				Field:  VehicleIDField,
				Kind:   Text,
				Needed: true,
				Flags:  []Flag{{Name: "vehicle", Hint: "the vehicle to link or unlink"}},
			},
			{
				Field:  TelemetryStateField,
				Kind:   Word,
				Words:  []string{"online", "offline"},
				Needed: true,
				Flags:  []Flag{{Name: "state"}},
			},
		},
	},
	{
		Kind: demoaction.SetPosition,
		Values: []Value{
			{
				Field:  VehicleIDField,
				Kind:   Text,
				Needed: true,
				Flags:  []Flag{{Name: "vehicle", Hint: "the vehicle to move"}},
			},
			{
				Field:  PositionField,
				Kind:   Point,
				Needed: true,
				Flags: []Flag{
					{Name: "longitude", Hint: "the longitude to put it at"},
					{Name: "latitude", Hint: "the latitude to put it at"},
				},
			},
		},
	},
	{
		Kind: demoaction.SetEnergyRemaining,
		Values: []Value{
			{
				Field:  VehicleIDField,
				Kind:   Text,
				Needed: true,
				Flags:  []Flag{{Name: "vehicle", Hint: "the vehicle whose reserve is stated"}},
			},
			{
				Field:  SourceKindField,
				Kind:   Word,
				Words:  []string{"battery", "gasoline", "diesel", "lpg", "cng"},
				Needed: true,
				Flags:  []Flag{{Name: "source"}},
			},
			{
				Field:  RemainingField,
				Kind:   Number,
				Needed: true,
				Flags:  []Flag{{Name: "remaining", Hint: "what the source is left holding"}},
			},
		},
	},
	{
		Kind: demoaction.MarkServiced,
		Values: []Value{
			{
				Field:  VehicleIDField,
				Kind:   Text,
				Needed: true,
				Flags:  []Flag{{Name: "vehicle", Hint: "the vehicle to return to service"}},
			},
		},
	},
	{
		Kind: demoaction.SetNextPaymentOutcome,
		Values: []Value{
			{
				Field:  RentalIDField,
				Kind:   Text,
				Needed: true,
				Flags:  []Flag{{Name: "rental", Hint: "the ride whose next attempt is decided"}},
			},
			{
				Field:  OutcomeField,
				Kind:   Word,
				Words:  []string{"paid", "failed"},
				Needed: true,
				Flags:  []Flag{{Name: "outcome"}},
			},
		},
	},
	{
		Kind:     demoaction.DropNextResponseAfterAccept,
		Terminal: "drop-next-response",
		Mail:     true,
	},
}

// Actions are every set-to-value command this build applies, in the order they are declared.
func Actions() []Action { return slices.Clone(actions[:]) }

// ActionOf answers the action the contract names, and reports whether this build applies it.
func ActionOf(name string) (Action, bool) {
	return actionMatching(func(action Action) bool { return string(action.Kind) == name })
}

// ActionNamed answers the action a terminal names, and reports whether this build applies it.
func ActionNamed(command string) (Action, bool) {
	return actionMatching(func(action Action) bool { return action.Command() == command })
}

func actionMatching(matches func(Action) bool) (Action, bool) {
	for _, action := range actions {
		if matches(action) {
			return action, true
		}
	}
	return Action{}, false
}

// Commands are the subcommands a terminal applies, in the order they are declared, which is the list
// its usage line offers.
func Commands() []string {
	commands := make([]string, 0, len(actions))
	for _, action := range actions {
		commands = append(commands, action.Command())
	}
	return commands
}

// checked refuses what either side must refuse: a value made of the wrong number of numbers, one the
// action cannot be applied without that was not stated, and a value of the wrong kind.
func (v Value) checked(stated []string) ([]string, error) {
	if len(stated) != len(v.Flags) {
		return nil, fmt.Errorf("the %s of a demonstration request is %d values", v.Field, len(v.Flags))
	}
	if v.Needed && slices.Contains(stated, "") {
		return nil, fmt.Errorf("%s is required", v.statedAs())
	}
	return stated, v.checkKind(stated)
}

func (v Value) checkKind(stated []string) error {
	switch v.Kind {
	case Word:
		if !slices.Contains(v.Words, stated[0]) {
			return fmt.Errorf("%s is %s", v.statedAs(), v.words())
		}
	case Number:
		if _, err := strconv.ParseFloat(stated[0], 64); err != nil {
			return fmt.Errorf("%s is a number: %w", v.statedAs(), err)
		}
	}
	return nil
}

// statedAs names the terminal's flags for this value, which is what a refusal tells a caller to fix.
func (v Value) statedAs() string {
	names := make([]string, 0, len(v.Flags))
	for _, flag := range v.Flags {
		names = append(names, "--"+flag.Name)
	}
	return strings.Join(names, ", ")
}

// hintOf is what a terminal says about one flag: the words a word may be, or what the flag carries.
func (v Value) hintOf(flag Flag) string {
	if v.Kind == Word {
		return v.words()
	}
	return flag.Hint
}

// words spells the set a word may come from the way a person reads a list.
func (v Value) words() string {
	if len(v.Words) < 2 {
		return strings.Join(v.Words, "")
	}
	return strings.Join(v.Words[:len(v.Words)-1], ", ") + " or " + v.Words[len(v.Words)-1]
}
