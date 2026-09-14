package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
	"github.com/Alisher24/CarSharing/backend/internal/tariffs"
)

// reservedAt is the one moment every case below is decided at, so an answer and the rental it
// describes are compared against the same instant.
var reservedAt = time.Date(2026, time.September, 13, 7, 15, 30, 123456000, time.UTC)

// frozenTariff is the price list a reservation below was made under. It deliberately differs from
// the catalog the cases publish, because an answer must state the conditions it stored rather than
// the ones that happen to be on offer now.
func frozenTariff() tariffs.Tariff {
	return tariffs.Tariff{
		ID:                               "01994342-6ba7-7000-8000-000300000001",
		Currency:                         "KGS",
		BillingPolicy:                    "per_mode_started_minute_v1",
		DrivingRateTyiynPerStartedMinute: 1234,
		PausedRateTyiynPerStartedMinute:  321,
		Version:                          7,
	}
}

func reservedRental() rentals.Rental {
	return rentals.Rental{
		ID:         "01994342-6ba7-7000-8000-000400000001",
		VehicleID:  "01994342-6ba7-7000-8000-000100000001",
		Stage:      stage.Reserved,
		Version:    1,
		ReservedAt: reservedAt,
		ExpiresAt:  reservedAt.Add(rentals.ReservationLifetime),
		Tariff:     frozenTariff(),
	}
}

// heldVehicle is the vehicle a reservation holds, as the command read it after making the change.
func heldVehicle() fleet.Vehicle {
	return catalogVehicle(reservedAt, "01994342-6ba7-7000-8000-000100000001", 9000, stage.Reserved)
}

func TestReservedAnswerPublishesTheStoredConditions(t *testing.T) {
	response, err := reserveRender(context.Background(), rentals.Outcome{
		Rental:  reservedRental(),
		Vehicle: heldVehicle(),
		Moment:  reservedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusCreated {
		t.Fatalf("reserving answered %d: %s", response.Status, response.Body)
	}

	var body servedapi.ReserveResult
	if err = json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.ServerTime != timestamp.Format(reservedAt) {
		t.Fatalf("the answer states %s", body.ServerTime)
	}
	if body.Rental.ExpiresAt != timestamp.Format(reservedAt.Add(rentals.ReservationLifetime)) {
		t.Fatalf("the deadline is %s", body.Rental.ExpiresAt)
	}
	if body.Rental.TariffSnapshot.DrivingRateTyiynPerStartedMinute != exactInteger(
		frozenTariff().DrivingRateTyiynPerStartedMinute) {
		t.Fatal("the answer published a rate other than the stored one")
	}
	if body.Rental.State != servedapi.ReservedRentalStateReserved {
		t.Fatalf("the rental is published as %s", body.Rental.State)
	}
	// The vehicle is published as the reservation left it rather than as it stood before the
	// command, which is what makes the public map and this answer agree.
	reserved, err := body.Rental.Vehicle.AsReservedVehicle()
	if err != nil {
		t.Fatal(err)
	}
	if reserved.Version != exactInteger(heldVehicle().Version) {
		t.Fatal("the answer published another version of the vehicle")
	}
}

func TestExhaustedAllowanceIsRefusedWithTheMomentItReturns(t *testing.T) {
	returns := reservedAt.Add(10*time.Hour + 44*time.Minute + 30*time.Second)
	response, err := refusalRender(context.Background(), reserveOperation, rentals.Refusal{
		Kind:  rentals.DailyLimitReached,
		Limit: rentals.DailyLimit{Available: false, ResetsAt: returns},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusConflict {
		t.Fatalf("the refusal answered %d", response.Status)
	}

	var body servedapi.ApiError
	if err = json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != servedapi.DAILYLIMITREACHED {
		t.Fatalf("the refusal carries %s", body.Code)
	}
	if body.Details == nil {
		t.Fatal("the refusal carries no moment to display")
	}
	details, err := body.Details.AsDailyLimitDetails()
	if err != nil {
		t.Fatal(err)
	}
	if details.DailyLimit.Available || details.DailyLimit.ResetsAt != timestamp.Format(returns) {
		t.Fatalf("the refusal states %+v", details.DailyLimit)
	}
}

func TestUnsuitableVehicleIsRefusedWithTheCatalogReasons(t *testing.T) {
	response, err := refusalRender(context.Background(), reserveOperation, rentals.Refusal{
		Kind:               rentals.VehicleUnavailable,
		UnavailableReasons: []fleet.UnavailableReason{fleet.InsufficientEnergy, fleet.TelemetryStale},
	})
	if err != nil {
		t.Fatal(err)
	}

	var body servedapi.ApiError
	if err = json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != servedapi.VEHICLEUNAVAILABLE {
		t.Fatalf("the refusal carries %s", body.Code)
	}
	details, err := body.Details.AsUnavailableDetails()
	if err != nil {
		t.Fatal(err)
	}
	if len(details.UnavailableReasons) != 2 || details.UnavailableReasons[0] != servedapi.InsufficientEnergy {
		t.Fatalf("the refusal states %v", details.UnavailableReasons)
	}
}

func TestCurrentAnswerCarriesTheAllowanceWithNoRental(t *testing.T) {
	returns := reservedAt.Add(time.Hour)
	body, err := currentSnapshot(rentals.Current{
		Moment: reservedAt,
		Limit:  rentals.DailyLimit{Available: false, ResetsAt: returns},
	})
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var answer servedapi.NoCurrentRental
	if err = json.Unmarshal(encoded, &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Kind != servedapi.None || answer.ServerTime != timestamp.Format(reservedAt) {
		t.Fatalf("the answer is %+v", answer)
	}
	// The allowance is answered on its own: a client displays it without inferring it from having
	// no rental.
	if answer.DailyLimit.Available || answer.DailyLimit.ResetsAt != timestamp.Format(returns) {
		t.Fatalf("the answer states the allowance as %+v", answer.DailyLimit)
	}
}

func TestCurrentAnswerPublishesTheLiveRentalAndTheVehicle(t *testing.T) {
	body, err := currentSnapshot(rentals.Current{
		Moment:  reservedAt,
		Rental:  rentalPointer(reservedRental()),
		Vehicle: heldVehicle(),
		Limit:   rentals.DailyLimit{Available: false, ResetsAt: reservedAt.Add(time.Hour)},
	})
	if err != nil {
		t.Fatal(err)
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var answer servedapi.CurrentRental
	if err = json.Unmarshal(encoded, &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Kind != servedapi.CurrentRentalKindRental {
		t.Fatalf("the answer is %+v", answer)
	}
	published, err := answer.Rental.AsReservedRental()
	if err != nil {
		t.Fatal(err)
	}
	if published.Id != reservedRental().ID {
		t.Fatal("the answer published another rental")
	}
	if _, err = published.Vehicle.AsReservedVehicle(); err != nil {
		t.Fatal("the vehicle of a reservation is not published as reserved")
	}
}

func TestCancelledRentalIsPublishedAsCancelled(t *testing.T) {
	cancelled := reservedRental()
	cancelled.Stage = stage.Cancelled
	cancelled.Version = 2
	ended := reservedAt.Add(time.Minute)
	cancelled.EndedAt = &ended

	response, err := cancelRender(context.Background(), rentals.Outcome{
		Rental:  cancelled,
		Vehicle: catalogVehicle(reservedAt, cancelled.VehicleID, 9000, stage.NotHeld),
		Moment:  ended,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusOK {
		t.Fatalf("cancelling answered %d: %s", response.Status, response.Body)
	}

	var body servedapi.RentalCommandResult
	if err = json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	published, err := body.Rental.AsCancelledRental()
	if err != nil {
		t.Fatal(err)
	}
	if published.CancelledAt != timestamp.Format(ended) {
		t.Fatalf("the cancellation states %s", published.CancelledAt)
	}
	if published.Version != exactInteger(2) {
		t.Fatal("the answer published another version of the rental")
	}
}

func rentalPointer(rental rentals.Rental) *rentals.Rental { return &rental }
