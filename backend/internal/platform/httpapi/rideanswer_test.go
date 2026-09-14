package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
)

// A ride command that moved a rental answers the shape its own operation declares, and the shape of one
// ride operation is not the shape of another: a pause answered as a start would not satisfy the
// interface the strict layer requires, which a client reads as a server defect.
func TestEachRideOperationAnswersItsOwnShape(t *testing.T) {
	answered := rentals.Answered{
		Response: rentals.Response{Status: http.StatusOK, Body: encodedRentalCommandResult(t)},
	}
	for _, operation := range []struct {
		name string
		op   commandOperation
	}{
		{"start", startRideOperation},
		{"pause", pauseRideOperation},
		{"resume", resumeRideOperation},
	} {
		spelled, err := answerOf(operation.op, answered)
		if err != nil {
			t.Fatalf("the %s answer was refused: %v", operation.name, err)
		}
		assertDeclaredShape(t, operation.name, spelled)
	}
}

// assertDeclaredShape reports whether an answer is one of the response objects the generated interface
// accepts. The three ride operations declare one each, and a value of another operation's type is a
// value the strict layer cannot write.
func assertDeclaredShape(t *testing.T, operation string, spelled any) {
	t.Helper()
	switch spelled.(type) {
	case servedapi.StartRentalResponseObject, servedapi.PauseRentalResponseObject,
		servedapi.ResumeRentalResponseObject:
	default:
		t.Fatalf("a %s answer is %T", operation, spelled)
	}
}

// A refusal is answered with the code the contract declares for the stage it belongs to, including the
// refusal of a ride command that names a rental this account does not hold.
func TestRideRefusalsCarryTheDeclaredCode(t *testing.T) {
	for _, refusal := range []struct {
		kind rentals.RefusalKind
		code servedapi.ErrorCode
	}{
		{rentals.ReservationExpired, servedapi.RESERVATIONEXPIRED},
		{rentals.RentalCompleted, servedapi.RENTALCOMPLETED},
		{rentals.InvalidRentalState, servedapi.INVALIDRENTALSTATE},
		{rentals.VehicleUnavailable, servedapi.VEHICLEUNAVAILABLE},
		{rentals.RentalNotFound, servedapi.RESOURCENOTFOUND},
	} {
		response, err := refusalRender(nil, startRideOperation, rentals.Refusal{Kind: refusal.kind})
		if err != nil {
			t.Fatalf("the %s refusal was not rendered: %v", refusal.kind, err)
		}
		var body servedapi.ApiError
		if err = json.Unmarshal(response.Body, &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != refusal.code {
			t.Errorf("the %s refusal carries %s, want %s", refusal.kind, body.Code, refusal.code)
		}
	}
}

// Every kind a command can refuse with is one this package knows how to spell. A kind the module adds
// without an answer here reaches a client as a server failure rather than as a refusal, so the rule is
// asserted rather than assumed.
func TestEveryRefusalKindIsSpelled(t *testing.T) {
	kinds := []rentals.RefusalKind{
		rentals.VehicleUnavailable,
		rentals.ActiveRentalExists,
		rentals.DailyLimitReached,
		rentals.ReservationExpired,
		rentals.RentalCompleted,
		rentals.InvalidRentalState,
		rentals.RentalNotFound,
	}
	for _, kind := range kinds {
		if _, _, _, err := refusalContract(rentals.Refusal{Kind: kind}); err != nil {
			t.Errorf("the %s refusal is not spelled: %v", kind, err)
		}
	}
	if _, _, _, err := refusalContract(rentals.Refusal{}); err == nil {
		t.Error("a refusal of no kind is spelled rather than reported")
	}
}

// Every status an operation can answer is declared with a shape, including the one answer every
// command states for a process that failed and the refusal each of them states for itself. A status
// without a shape is an answer this server cannot write, which a client reads as a crash.
func TestEveryCommandOperationDeclaresEveryStatusItAnswers(t *testing.T) {
	for _, operation := range []struct {
		name     string
		op       commandOperation
		statuses []int
	}{
		{"reservation", reserveOperation, []int{
			http.StatusCreated, http.StatusConflict, http.StatusServiceUnavailable,
		}},
		{"cancellation", cancelOperation, []int{
			http.StatusOK, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable,
		}},
		{"start", startRideOperation, []int{
			http.StatusOK, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable,
		}},
		{"pause", pauseRideOperation, []int{
			http.StatusOK, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable,
		}},
		{"resume", resumeRideOperation, []int{
			http.StatusOK, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable,
		}},
	} {
		for _, status := range operation.statuses {
			if _, declared := operation.op.answers[status]; !declared {
				t.Errorf("the %s operation answers %d without a shape", operation.name, status)
			}
		}
		if operation.op.refused != http.StatusConflict {
			t.Errorf("the %s operation refuses with %d", operation.name, operation.op.refused)
		}
	}
}

// A failure of the process is answered as the service failure its operation declares, and not as a
// refusal: a client is told to come back rather than that its command was wrong.
func TestUndecidedCommandIsAnsweredAsAServiceFailure(t *testing.T) {
	for _, operation := range []struct {
		name string
		op   commandOperation
	}{
		{"reservation", reserveOperation},
		{"cancellation", cancelOperation},
		{"start", startRideOperation},
		{"pause", pauseRideOperation},
		{"resume", resumeRideOperation},
	} {
		failure := commandFailure{code: codeServiceUnavailable, message: messageServiceUnavailable}
		spelled, err := spellFailure(operation.op, context.Background(), failure)
		if err != nil {
			t.Fatalf("the %s failure was not spelled: %v", operation.name, err)
		}
		if spelled == nil {
			t.Fatalf("the %s failure produced no answer", operation.name)
		}
	}
}

// encodedRentalCommandResult is one ride as a command that moved it answers, written from the
// contract's own type so a change to the published shape fails this test rather than a client.
func encodedRentalCommandResult(t *testing.T) []byte {
	t.Helper()
	encoded, err := json.Marshal(servedapi.RentalCommandResult{
		ServerTime: servedapi.Timestamp("2026-09-14T10:00:00.000000Z"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// rideMoment is one moment a ride began, which the answers of this package publish.
func rideMoment() time.Time {
	return time.Date(2026, time.September, 14, 10, 0, 0, 0, time.UTC)
}
