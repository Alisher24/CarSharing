package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/platform/cursor"
	"github.com/Alisher24/CarSharing/backend/internal/rentals"
	"github.com/google/uuid"
)

// getRidesOperation is the name the signed cursors of this collection are bound to: the operation's
// own identifier in the contract.
const getRidesOperation = "getRides"

// rideCollectionHandlers answers the history of the caller's own finished rides. It is a separate
// handler from the ride commands because it answers a different question: what the account has
// already ridden, rather than what it may do with the ride it is in.
type rideCollectionHandlers struct {
	rides   Reservations
	cursors *cursor.Signer
}

func newRideCollectionHandlers(rides Reservations, cursors *cursor.Signer) (rideCollectionHandlers, error) {
	if rides == nil {
		return rideCollectionHandlers{}, fmt.Errorf("%w: ride history", ErrIncompleteApplication)
	}
	if cursors == nil {
		return rideCollectionHandlers{}, fmt.Errorf("%w: cursor signer", ErrIncompleteApplication)
	}
	return rideCollectionHandlers{rides: rides, cursors: cursors}, nil
}

// GetRides answers one page of the caller's own finished rides, newest first. The owner comes from
// the session and never from a parameter, so another account's history cannot be asked for.
//
// A ride no invoice was written for is answered as a failure of this server rather than left out of
// the page: the contract publishes the invoice of every ride it lists, and a history that silently
// loses a ride is worse than a refusal that can be seen.
func (h rideCollectionHandlers) GetRides(
	ctx context.Context, request servedapi.GetRidesRequestObject,
) (servedapi.GetRidesResponseObject, error) {
	caller, signedIn := callerOf(ctx)
	if !signedIn {
		return servedapi.GetRides401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}
	limit := pageLimitOf(request.Params.Limit, cursor.PageSize)

	after, err := h.positionOf(request.Params.Cursor, caller, limit)
	if err != nil {
		return servedapi.GetRides400JSONResponse{
			Body: apiErrorBody(ctx, codeInvalidCursor, messageInvalidCursor),
		}, nil
	}
	page, err := h.rides.Rides(ctx, caller, after, limit)
	if errors.Is(err, rentals.ErrRideWithoutInvoice) {
		slog.ErrorContext(ctx, "a finished ride has no invoice to publish", "error", err)
		return servedapi.GetRides500JSONResponse{Body: internalError(ctx)}, nil
	}
	if err != nil {
		slog.ErrorContext(ctx, "the ride history could not be read", "error", err)
		return servedapi.GetRides503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	body, err := h.collectionBody(page, caller, limit)
	if err != nil {
		return nil, err
	}
	return servedapi.GetRides200JSONResponse{Body: body}, nil
}

// positionOf reads the position a presented cursor names, in the vocabulary of the module that pages
// the history.
func (h rideCollectionHandlers) positionOf(
	presented *servedapi.Cursor, owner uuid.UUID, limit int,
) (*cursor.Position, error) {
	return pagePositionOf(h.cursors, presented, accountPageScope(getRidesOperation, owner, limit))
}

// collectionBody renders one page of the history, with the cursor that reads the page after it. A
// last or empty page carries no cursor: there is nothing to continue from, and the contract
// publishes that as a null cursor.
func (h rideCollectionHandlers) collectionBody(
	page rentals.RidePage, owner uuid.UUID, limit int,
) (servedapi.RideCollection, error) {
	items := make([]servedapi.RideSummary, 0, len(page.Rides))
	for _, ride := range page.Rides {
		item, err := rideSummaryBody(ride)
		if err != nil {
			return servedapi.RideCollection{}, err
		}
		items = append(items, item)
	}
	body := servedapi.RideCollection{Items: items}
	if page.Next == nil {
		return body, nil
	}
	issued, err := h.cursors.Issue(*page.Next, accountPageScope(getRidesOperation, owner, limit))
	if err != nil {
		return servedapi.RideCollection{}, err
	}
	body.NextCursor = &issued
	return body, nil
}

func registerRideCollectionHandlers(served *server, dependencies Dependencies) error {
	var err error
	served.rideCollectionHandlers, err = newRideCollectionHandlers(dependencies.Reservations, dependencies.Cursors)
	return err
}
