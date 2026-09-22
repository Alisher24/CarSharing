package httpapi

import (
	"fmt"
	"net/http"

	internalapi "github.com/Alisher24/CarSharing/backend/internal/contracts/internalapi"
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
)

type refusalDeclaration struct {
	code    servedapi.ErrorCode
	status  int
	message string
}

var publicRefusals = map[rentals.RefusalKind]refusalDeclaration{
	rentals.VehicleUnavailable: {
		code:    servedapi.VEHICLEUNAVAILABLE,
		status:  http.StatusConflict,
		message: messageVehicleUnavailable,
	},
	rentals.ActiveRentalExists: {
		code:    servedapi.ACTIVERENTALEXISTS,
		status:  http.StatusConflict,
		message: messageActiveRentalExists,
	},
	rentals.DailyLimitReached: {
		code:    servedapi.DAILYLIMITREACHED,
		status:  http.StatusConflict,
		message: messageDailyLimitReached,
	},
	rentals.ReservationExpired: {
		code:    servedapi.RESERVATIONEXPIRED,
		status:  http.StatusConflict,
		message: messageReservationExpired,
	},
	rentals.RentalCompleted: {
		code:    servedapi.RENTALCOMPLETED,
		status:  http.StatusConflict,
		message: messageRentalCompleted,
	},
	rentals.InvalidRentalState: {
		code:    servedapi.INVALIDRENTALSTATE,
		status:  http.StatusConflict,
		message: messageInvalidRentalState,
	},
	rentals.OutsideServiceZone: {
		code:    servedapi.OUTSIDESERVICEZONE,
		status:  http.StatusConflict,
		message: messageOutsideServiceZone,
	},
	rentals.TelemetryStale: {
		code:    servedapi.TELEMETRYSTALE,
		status:  http.StatusConflict,
		message: messageTelemetryStale,
	},
	rentals.OutstandingInvoice: {
		code:    servedapi.OUTSTANDINGINVOICE,
		status:  http.StatusConflict,
		message: messageOutstandingInvoice,
	},
	rentals.PaymentInProgress: {
		code:    servedapi.PAYMENTINPROGRESS,
		status:  http.StatusConflict,
		message: messagePaymentInProgress,
	},
	rentals.RentalNotFound:  resourceNotFoundRefusal,
	rentals.InvoiceNotFound: resourceNotFoundRefusal,
}

var internalRefusals = map[rentals.RefusalKind]refusalDeclaration{
	rentals.VehicleInUse: {
		code:    servedapi.ErrorCode(internalapi.VEHICLEINUSE),
		status:  http.StatusConflict,
		message: messageVehicleInUse,
	},
	rentals.SourceNotCarried: {
		code:    servedapi.ErrorCode(internalapi.SOURCENOTCARRIED),
		status:  http.StatusConflict,
		message: messageSourceNotCarried,
	},
	rentals.SourceCapacityExceeded: {
		code:    servedapi.ErrorCode(internalapi.SOURCECAPACITYEXCEEDED),
		status:  http.StatusConflict,
		message: messageSourceCapacityExceeded,
	},
	rentals.VehicleNotFound: resourceNotFoundRefusal,
	rentals.RentalNotFound:  resourceNotFoundRefusal,
}

var resourceNotFoundRefusal = refusalDeclaration{codeResourceNotFound, http.StatusNotFound, messageResourceNotFound}

func refusalContract(
	refusal rentals.Refusal, declarations map[rentals.RefusalKind]refusalDeclaration,
) (servedapi.ErrorCode, int, string, error) {
	declaration, known := declarations[refusal.Kind]
	if !known {
		return "", 0, "", fmt.Errorf("the rentals module refused with an unknown kind %q", refusal.Kind)
	}
	return declaration.code, declaration.status, declaration.message, nil
}
