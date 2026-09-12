package httpapi

import (
	"encoding/json"
	"sort"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
)

// violation names one field or parameter the request got wrong, so a client can correct the
// request without guessing which part the contract rejected.
type violation struct {
	Location  string  `json:"location"`
	Pointer   *string `json:"pointer,omitempty"`
	Parameter string  `json:"parameter,omitempty"`
	Code      string  `json:"code"`
	Message   string  `json:"message"`
}

// constraintsKey addresses the positional constraints carried from requireJSONRequestBody to the
// handlers that report them.
type constraintsKey struct{}

// locationOrder is the order violations are reported in: the body first, then the parameters from
// the most to the least specific to the resource.
var locationOrder = map[string]int{
	locationBody:   0,
	locationQuery:  1,
	locationPath:   2,
	locationHeader: 3,
}

func bodyViolation(pointer, code, message string) violation {
	return violation{Location: locationBody, Pointer: &pointer, Code: code, Message: message}
}

// violationDetails renders violations into the free-form details object of the error contract, in a
// stable order and without repeats so that an identical request always produces an identical body.
func violationDetails(violations []violation) *servedapi.ApiError_Details {
	sortViolations(violations)
	data, _ := json.Marshal(struct {
		Violations []violation `json:"violations"`
	}{dedupeViolations(violations)})
	details := servedapi.ApiError_Details{}
	_ = details.UnmarshalJSON(data)
	return &details
}

// sortViolations orders violations by location, then by the field or parameter they address, then
// by code, so that neither map iteration nor validator ordering can vary the response.
func sortViolations(violations []violation) {
	sort.Slice(violations, lessViolation(violations))
}

// lessViolation is the order sortViolations applies, expressed as a comparison of two violations.
func lessViolation(violations []violation) func(int, int) bool {
	return func(i, j int) bool {
		left, right := violations[i], violations[j]
		switch {
		case locationOrder[left.Location] != locationOrder[right.Location]:
			return locationOrder[left.Location] < locationOrder[right.Location]
		case violationTarget(left) != violationTarget(right):
			return violationTarget(left) < violationTarget(right)
		default:
			return left.Code < right.Code
		}
	}
}

// dedupeViolations drops repeats of the same code on the same target. It requires sorted input, so
// that repeats are adjacent, and reuses the backing array because the sorted slice is not read again.
func dedupeViolations(violations []violation) []violation {
	unique := violations[:0]
	for _, candidate := range violations {
		if len(unique) > 0 && sameViolation(unique[len(unique)-1], candidate) {
			continue
		}
		unique = append(unique, candidate)
	}
	return unique
}

func sameViolation(left, right violation) bool {
	return left.Location == right.Location &&
		violationTarget(left) == violationTarget(right) &&
		left.Code == right.Code
}

// violationTarget is what the violation addresses: a JSON pointer into the body, or a parameter name.
func violationTarget(failed violation) string {
	if failed.Pointer != nil {
		return *failed.Pointer
	}
	return failed.Parameter
}
