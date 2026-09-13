package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/platform/timestamp"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
)

// Messages the reservation operations answer their domain refusals with. A client translates the
// codes it knows, so these describe the outcome rather than address the person.
const (
	messageVehicleUnavailable    = "Vehicle is not available"
	messageActiveRentalExists    = "An active rental already exists"
	messageDailyLimitReached     = "The free reservation of this day has been used"
	messageReservationExpired    = "The reservation has expired"
	messageRentalCompleted       = "The rental has already been completed"
	messageInvalidRentalState    = "The rental does not allow this operation"
	messageIdempotencyConflict   = "The command key already answered another command"
	messageIdempotencyInProgress = "The same command is still being processed"
)

// reserveRender spells the outcome of a reservation command as the answer its operation declares.
func reserveRender(ctx context.Context, outcome rentals.Outcome) (rentals.Response, error) {
	if outcome.Refused() {
		return refusalResponse(ctx, outcome.Refusal)
	}
	rental, err := reservedRentalBody(outcome)
	if err != nil {
		return rentals.Response{}, err
	}
	return encoded(http.StatusCreated, servedapi.ReserveResult{
		ServerTime: timestamp.Format(outcome.Moment),
		Rental:     rental,
	})
}

// cancelRender spells the outcome of a cancellation as the answer its operation declares.
func cancelRender(ctx context.Context, outcome rentals.Outcome) (rentals.Response, error) {
	if outcome.Refused() {
		return refusalResponse(ctx, outcome.Refusal)
	}
	rental, err := rentalBody(outcome.Rental, outcome.Vehicle, outcome.Moment)
	if err != nil {
		return rentals.Response{}, err
	}
	return encoded(http.StatusOK, servedapi.RentalCommandResult{
		ServerTime: timestamp.Format(outcome.Moment),
		Rental:     rental,
	})
}

// refusalResponse renders a domain refusal. The code carries its own status, and the details a
// client displays travel in the error envelope rather than beside it.
func refusalResponse(ctx context.Context, refusal rentals.Refusal) (rentals.Response, error) {
	code, status, message, err := refusalContract(refusal)
	if err != nil {
		return rentals.Response{}, err
	}
	body := apiErrorBody(ctx, code, message)
	details, err := refusalDetails(refusal)
	if err != nil {
		return rentals.Response{}, err
	}
	body.Details = details
	return encoded(status, body)
}

// refusalContract names the code, status and message of a refusal. Every kind is one the contract
// declares for the operation that can produce it.
func refusalContract(refusal rentals.Refusal) (servedapi.ErrorCode, int, string, error) {
	switch refusal.Kind {
	case rentals.VehicleUnavailable:
		return servedapi.VEHICLEUNAVAILABLE, http.StatusConflict, messageVehicleUnavailable, nil
	case rentals.ActiveRentalExists:
		return servedapi.ACTIVERENTALEXISTS, http.StatusConflict, messageActiveRentalExists, nil
	case rentals.DailyLimitReached:
		return servedapi.DAILYLIMITREACHED, http.StatusConflict, messageDailyLimitReached, nil
	case rentals.ReservationExpired:
		return servedapi.RESERVATIONEXPIRED, http.StatusConflict, messageReservationExpired, nil
	case rentals.RentalCompleted:
		return servedapi.RENTALCOMPLETED, http.StatusConflict, messageRentalCompleted, nil
	case rentals.InvalidRentalState:
		return servedapi.INVALIDRENTALSTATE, http.StatusConflict, messageInvalidRentalState, nil
	case rentals.RentalNotFound:
		return codeResourceNotFound, http.StatusNotFound, messageResourceNotFound, nil
	default:
		return "", 0, "", fmt.Errorf("the rentals module refused with an unknown kind %q", refusal.Kind)
	}
}

// refusalDetails renders what displaying a refusal needs: why a vehicle cannot be used, or when the
// day's allowance returns. A refusal the contract does not describe in detail carries none.
func refusalDetails(refusal rentals.Refusal) (*servedapi.ApiError_Details, error) {
	switch refusal.Kind {
	case rentals.VehicleUnavailable:
		if len(refusal.UnavailableReasons) == 0 {
			return nil, nil
		}
		details := servedapi.ApiError_Details{}
		reasons := make([]servedapi.UnavailableReason, 0, len(refusal.UnavailableReasons))
		for _, reason := range refusal.UnavailableReasons {
			reasons = append(reasons, servedapi.UnavailableReason(reason))
		}
		err := details.FromUnavailableDetails(servedapi.UnavailableDetails{UnavailableReasons: reasons})
		return &details, err
	case rentals.DailyLimitReached:
		details := servedapi.ApiError_Details{}
		err := details.FromDailyLimitDetails(servedapi.DailyLimitDetails{
			DailyLimit: dailyLimitState(refusal.Limit),
		})
		return &details, err
	default:
		return nil, nil
	}
}

// dailyLimitState publishes the day's allowance with the moment it returns.
func dailyLimitState(limit rentals.DailyLimit) servedapi.DailyLimitState {
	return servedapi.DailyLimitState{
		Available: limit.Available,
		ResetsAt:  timestamp.Format(limit.ResetsAt),
	}
}

// currentSnapshot renders the current-rental read. Both shapes carry the moment of the read and the
// day's allowance, so a client displays the allowance without inferring it from the absence of a
// rental.
func currentSnapshot(current rentals.Current) (servedapi.CurrentSnapshot, error) {
	limit := dailyLimitState(current.Limit)
	serverTime := timestamp.Format(current.Moment)

	var body servedapi.CurrentSnapshot
	if current.Rental == nil {
		return body, body.FromNoCurrentRental(servedapi.NoCurrentRental{
			Kind:       servedapi.None,
			ServerTime: serverTime,
			DailyLimit: limit,
		})
	}
	rental, err := rentalBody(*current.Rental, current.Vehicle, current.Moment)
	if err != nil {
		return servedapi.CurrentSnapshot{}, err
	}
	return body, body.FromCurrentRental(servedapi.CurrentRental{
		Kind:       servedapi.CurrentRentalKindRental,
		ServerTime: serverTime,
		Rental:     rental,
		DailyLimit: limit,
	})
}

// reservedRentalBody publishes a reservation together with the conditions it was made under. The
// vehicle is the one the command read after the change, so the answer shows the vehicle as held
// rather than as it stood before.
func reservedRentalBody(outcome rentals.Outcome) (servedapi.ReservedRental, error) {
	vehicle, err := publishedVehicleBody(outcome.Vehicle, outcome.Moment)
	if err != nil {
		return servedapi.ReservedRental{}, err
	}
	return servedapi.ReservedRental{
		Id:             outcome.Rental.ID,
		Vehicle:        vehicle,
		Version:        exactInteger(outcome.Rental.Version),
		ReservedAt:     timestamp.Format(outcome.Rental.ReservedAt),
		TariffSnapshot: tariffSnapshotBody(outcome.Rental),
		State:          servedapi.ReservedRentalStateReserved,
		ExpiresAt:      timestamp.Format(outcome.Rental.ExpiresAt),
	}, nil
}

// rentalBody publishes a rental in the shape its stage selects. A ride is published from the facts
// the rental itself records: this build records no interval and no mode change, so a ride that has
// started reports the moments it knows and the durations its records account for. Ride commands and
// the intervals they track belong to the tasks that own them.
func rentalBody(rental rentals.Rental, vehicle fleet.Vehicle, moment time.Time) (servedapi.Rental, error) {
	published, err := publishedVehicleBody(vehicle, moment)
	if err != nil {
		return servedapi.Rental{}, err
	}
	var body servedapi.Rental
	switch rental.Stage {
	case stage.Reserved:
		return body, body.FromReservedRental(servedapi.ReservedRental{
			Id:             rental.ID,
			Vehicle:        published,
			Version:        exactInteger(rental.Version),
			ReservedAt:     timestamp.Format(rental.ReservedAt),
			TariffSnapshot: tariffSnapshotBody(rental),
			State:          servedapi.ReservedRentalStateReserved,
			ExpiresAt:      timestamp.Format(rental.ExpiresAt),
		})
	case stage.Cancelled:
		return body, body.FromCancelledRental(servedapi.CancelledRental{
			Id:             rental.ID,
			Vehicle:        published,
			Version:        exactInteger(rental.Version),
			ReservedAt:     timestamp.Format(rental.ReservedAt),
			TariffSnapshot: tariffSnapshotBody(rental),
			State:          servedapi.Cancelled,
			CancelledAt:    timestamp.Format(endedAt(rental)),
		})
	case stage.Expired:
		return body, body.FromExpiredRental(servedapi.ExpiredRental{
			Id:             rental.ID,
			Vehicle:        published,
			Version:        exactInteger(rental.Version),
			ReservedAt:     timestamp.Format(rental.ReservedAt),
			TariffSnapshot: tariffSnapshotBody(rental),
			State:          servedapi.Expired,
			ExpiredAt:      timestamp.Format(endedAt(rental)),
		})
	default:
		return servedapi.Rental{}, fmt.Errorf("a rental in stage %q cannot be published yet", rental.Stage)
	}
}

// endedAt is the moment a rental released its vehicle. A stored rental that released it has one;
// reading it as the deadline keeps an unreadable row from publishing a moment nobody recorded.
func endedAt(rental rentals.Rental) time.Time {
	if rental.EndedAt != nil {
		return *rental.EndedAt
	}
	return rental.ExpiresAt
}

// tariffSnapshotBody publishes the conditions the rental was made under, which are the stored ones
// rather than the catalogue as it stands now.
func tariffSnapshotBody(rental rentals.Rental) servedapi.TariffSnapshot {
	return servedapi.TariffSnapshot{
		Id:                               rental.Tariff.ID,
		Currency:                         servedapi.TariffSnapshotCurrency(rental.Tariff.Currency),
		BillingPolicy:                    servedapi.TariffSnapshotBillingPolicy(rental.Tariff.BillingPolicy),
		DrivingRateTyiynPerStartedMinute: exactInteger(rental.Tariff.DrivingRateTyiynPerStartedMinute),
		PausedRateTyiynPerStartedMinute:  exactInteger(rental.Tariff.PausedRateTyiynPerStartedMinute),
		Version:                          exactInteger(rental.Tariff.Version),
	}
}

// publishedVehicleBody renders one vehicle in the state it stands in at the moment the answer
// states, so a vehicle the answer says is held is published as held.
func publishedVehicleBody(vehicle fleet.Vehicle, moment time.Time) (servedapi.Vehicle, error) {
	return vehicleBody(vehicle, vehicle.StateAt(moment), vehicle.TelemetryFreshnessAt(moment))
}

// encoded renders one contract body as the bytes the client receives and the module stores.
func encoded(status int, body any) (rentals.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return rentals.Response{}, err
	}
	return rentals.Response{Status: status, Body: data}, nil
}
