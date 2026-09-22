package democontrol

import (
	"flag"
	"fmt"
	"slices"
	"strconv"
)

// Declare declares one flag for every value of the action on a terminal's flag set, under the names
// this declaration states and with the hint it gives. The order is the order the values are declared
// in, so a terminal's flags read in the order its usage line does.
func (a Action) Declare(flags *flag.FlagSet) {
	for _, value := range a.Values {
		for _, named := range value.Flags {
			flags.String(named.Name, "", value.hintOf(named))
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
	request := Request{ActionField: string(a.Kind)}
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
