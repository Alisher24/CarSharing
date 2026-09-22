package democontrol

import (
	"encoding/json"
	"flag"
	"fmt"
	"slices"
	"strconv"
	"strings"
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

// Flag is one name a terminal states a value under, together with what the terminal says about it.
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

	// Words are the words a Word may be, in the order a terminal offers them.
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
	// Name is the identifier the contract publishes for the action, which is also the word the
	// internal surface dispatches on and the kind the module applies.
	Name string

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
	return strings.ReplaceAll(a.Name, "_", "-")
}

// actions is every set-to-value command this build applies, declared once. A new command is a new row:
// the terminal's flags and usage line and the surface's reading of a request all follow from it.
var actions = [...]Action{
	{
		Name: "set_telemetry_state",
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
				Flags:  []Flag{{Name: "state", Hint: "online or offline"}},
			},
		},
	},
	{
		Name: "set_position",
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
		Name: "set_energy_remaining",
		Values: []Value{
			{
				Field:  VehicleIDField,
				Kind:   Text,
				Needed: true,
				Flags:  []Flag{{Name: "vehicle", Hint: "the vehicle whose reserve is stated"}},
			},
			{
				Field:  SourceKindField,
				Kind:   Text,
				Needed: true,
				Flags:  []Flag{{Name: "source", Hint: "battery, gasoline, diesel, lpg or cng"}},
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
		Name: "mark_serviced",
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
		Name: "set_next_payment_outcome",
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
				Flags:  []Flag{{Name: "outcome", Hint: "paid or failed"}},
			},
		},
	},
	{
		Name:     "drop_next_response_after_accept",
		Terminal: "drop-next-response",
		Mail:     true,
	},
}

// Actions are every set-to-value command this build applies, in the order they are declared.
func Actions() []Action { return slices.Clone(actions[:]) }

// ActionOf answers the action the contract names, and reports whether this build applies it.
func ActionOf(name string) (Action, bool) {
	return actionMatching(func(action Action) bool { return action.Name == name })
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

// Declare declares one flag for every value of the action on a terminal's flag set, under the names
// and with the hints this declaration states. The order is the order the values are declared in, so a
// terminal's flags read in the order its usage line does.
func (a Action) Declare(flags *flag.FlagSet) {
	for _, value := range a.Values {
		for _, named := range value.Flags {
			flags.String(named.Name, "", named.Hint)
		}
	}
}

// StatedOf reads what a terminal stated of the action out of the flags that were declared for it. A
// flag that was not named reads as the empty string, which is how a value the action needs is told
// from one that was stated.
func (a Action) StatedOf(flags *flag.FlagSet) map[Field][]string {
	stated := make(map[Field][]string, len(a.Values))
	for _, value := range a.Values {
		for _, named := range value.Flags {
			stated[value.Field] = append(stated[value.Field], flags.Lookup(named.Name).Value.String())
		}
	}
	return stated
}

// Request writes what a terminal stated as the request the contract receives: the action it is, and
// every value it carries under the field the contract puts it in. A value the action needs that was
// not stated, and a word outside the declared set, are refused here rather than by the surface.
func (a Action) Request(stated map[Field][]string) (Request, error) {
	request := Request{ActionField: a.Name}
	for _, value := range a.Values {
		given := stated[value.Field]
		if !value.Needed && !slices.ContainsFunc(given, statedText) {
			continue
		}
		packed, err := value.Pack(given)
		if err != nil {
			return nil, err
		}
		request[string(value.Field)] = packed
	}
	return request, nil
}

func statedText(text string) bool { return text != "" }

// Stated is one demonstration request as a surface reads it: the action it names, the identifier the
// command is remembered by, and what each value of that action stated.
type Stated struct {
	Action   string
	ActionID string
	Values   map[Field][]string
}

// Read reads one request body as the action it names together with what that action's values stated.
// A body naming an action this build does not apply, or stating a value in a shape the declaration
// does not allow, is refused rather than half-read.
func Read(body []byte) (Stated, error) {
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return Stated{}, fmt.Errorf("a demonstration request is an object: %w", err)
	}
	action, err := statedAction(request)
	if err != nil {
		return Stated{}, err
	}
	stated := Stated{Action: action.Name, Values: map[Field][]string{}}
	stated.ActionID, _ = request[ActionIDField].(string)
	for _, value := range action.Values {
		carried, declared := request[string(value.Field)]
		if !declared || carried == nil {
			if value.Needed {
				return Stated{}, fmt.Errorf("the %s action states %s", action.Name, value.Field)
			}
			continue
		}
		values, err := value.Unpack(carried)
		if err != nil {
			return Stated{}, err
		}
		stated.Values[value.Field] = values
	}
	return stated, nil
}

func statedAction(request map[string]any) (Action, error) {
	name, _ := request[ActionField].(string)
	action, known := ActionOf(name)
	if !known {
		return Action{}, fmt.Errorf("the demonstration action %q is not one this build applies", name)
	}
	return action, nil
}

// Pack writes what a terminal stated as the field the request carries. A value the action needs that
// was not stated, a word outside the declared set and a number that is not one are refused here, so
// the terminal judges a statement by the declaration the surface reads it with.
func (v Value) Pack(stated []string) (any, error) {
	checked, err := v.checked(stated)
	if err != nil {
		return nil, err
	}
	if v.Kind != Point {
		return checked[0], nil
	}
	numbers := make([]any, 0, len(checked))
	for _, number := range checked {
		parsed, err := strconv.ParseFloat(number, 64)
		if err != nil {
			return nil, fmt.Errorf("%s is a number: %w", v.statedAs(), err)
		}
		numbers = append(numbers, parsed)
	}
	return map[string]any{"type": "Point", "coordinates": numbers}, nil
}

// Unpack reads the field of a request back as what a terminal stated, so the surface applies an action
// by the declaration the terminal wrote it with.
func (v Value) Unpack(carried any) ([]string, error) {
	if v.Kind != Point {
		text, isText := carried.(string)
		if !isText {
			return nil, fmt.Errorf("the %s of a demonstration request is text", v.Field)
		}
		return v.checked([]string{text})
	}
	point, isPoint := carried.(map[string]any)
	if !isPoint {
		return nil, fmt.Errorf("the %s of a demonstration request is a point", v.Field)
	}
	coordinates, areCoordinates := point["coordinates"].([]any)
	if !areCoordinates || len(coordinates) != len(v.Flags) {
		return nil, fmt.Errorf("the %s of a demonstration request is a point", v.Field)
	}
	stated := make([]string, 0, len(coordinates))
	for _, coordinate := range coordinates {
		number, isNumber := coordinate.(float64)
		if !isNumber {
			return nil, fmt.Errorf("the %s of a demonstration request is a point", v.Field)
		}
		stated = append(stated, strconv.FormatFloat(number, 'f', -1, 64))
	}
	return v.checked(stated)
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
			return fmt.Errorf("%s is %s", v.statedAs(), strings.Join(v.Words, " or "))
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
