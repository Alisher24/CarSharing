package democontrol

import (
	"encoding/json"
	"fmt"

	"github.com/Alisher24/CarSharing/backend/internal/demoaction"
)

// Stated is one demonstration request as a surface reads it: the action it names, the identifier the
// command is remembered by, and what each field of that action stated.
type Stated struct {
	Action   demoaction.Kind
	ActionID string

	// values are what the request carried, as the declaration of each field reads it.
	values map[Field]any
}

// point is the two numbers a point field carried, in the order the contract writes them.
type point struct {
	longitude float64
	latitude  float64
}

// Fields are the fields the request carried, in the order the action declares them.
func (s Stated) Fields() []Field {
	action, known := ActionOf(string(s.Action))
	if !known {
		return nil
	}
	fields := make([]Field, 0, len(action.Values))
	for _, value := range action.Values {
		if _, carried := s.values[value.Field]; carried {
			fields = append(fields, value.Field)
		}
	}
	return fields
}

// Text answers what a field of the request stated, for every field the declaration writes as text, a
// word or a number.
func (s Stated) Text(field Field) (string, error) {
	carried, declared := s.values[field]
	if !declared {
		return "", fmt.Errorf("the demonstration request states no %s", field)
	}
	text, isText := carried.(string)
	if !isText {
		return "", fmt.Errorf("the %s of a demonstration request is not text", field)
	}
	return text, nil
}

// Point answers the two numbers a point field stated.
func (s Stated) Point(field Field) (longitude, latitude float64, err error) {
	carried, declared := s.values[field]
	if !declared {
		return 0, 0, fmt.Errorf("the demonstration request states no %s", field)
	}
	at, isPoint := carried.(point)
	if !isPoint {
		return 0, 0, fmt.Errorf("the %s of a demonstration request is not a point", field)
	}
	return at.longitude, at.latitude, nil
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
	stated := Stated{Action: action.Kind, values: map[Field]any{}}
	stated.ActionID, _ = request[ActionIDField].(string)
	for _, value := range action.Values {
		carried, declared := request[string(value.Field)]
		if !declared || carried == nil {
			if value.Needed {
				return Stated{}, fmt.Errorf("the %s action states %s", action.Kind, value.Field)
			}
			continue
		}
		read, err := value.read(carried)
		if err != nil {
			return Stated{}, err
		}
		stated.values[value.Field] = read
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

// read reads the field of a request as the declaration says it is written: the text of a value stated
// as text, a word or a number, and the two numbers of a point. What the declaration does not allow is
// refused here, by the same words the terminal was judged by.
func (v Value) read(carried any) (any, error) {
	if v.Kind != Point {
		text, isText := carried.(string)
		if !isText {
			return nil, fmt.Errorf("the %s of a demonstration request is text", v.Field)
		}
		if _, err := v.checked([]string{text}); err != nil {
			return nil, err
		}
		return text, nil
	}
	shape, isPoint := carried.(map[string]any)
	if !isPoint {
		return nil, fmt.Errorf("the %s of a demonstration request is a point", v.Field)
	}
	coordinates, areCoordinates := shape["coordinates"].([]any)
	if !areCoordinates || len(coordinates) != len(v.Flags) {
		return nil, fmt.Errorf("the %s of a demonstration request is a point", v.Field)
	}
	at := point{}
	numbers := make([]float64, 0, len(coordinates))
	for _, coordinate := range coordinates {
		number, isNumber := coordinate.(float64)
		if !isNumber {
			return nil, fmt.Errorf("the %s of a demonstration request is a point", v.Field)
		}
		numbers = append(numbers, number)
	}
	at.longitude, at.latitude = numbers[0], numbers[1]
	return at, nil
}
