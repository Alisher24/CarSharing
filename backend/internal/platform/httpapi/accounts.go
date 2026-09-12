package httpapi

import (
	"context"
	"errors"

	"github.com/Alisher24/CarSharing/backend/internal/auth"
	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/Alisher24/CarSharing/backend/internal/platform/sessions"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// Messages the account operations answer failures with. They describe the outcome without telling
// a caller which half of a credential pair was wrong.
const (
	messageEmailAlreadyRegistered = "Email is already registered"
	messageInvalidCredentials     = "Invalid email or password"
	messageServiceUnavailable     = "Service unavailable"
)

// accounts answers the operations an account is created and proven through. Registration and
// sign-in each run as one transaction, so the user and the session that carries them reach storage
// together or not at all, and the cookie is written only once that transaction has committed.
type accounts struct {
	pool     *pgxpool.Pool
	sessions *sessions.Manager
	service  *auth.Service
	users    *auth.UserStore
}

func (a accounts) Register(
	ctx context.Context, request servedapi.RegisterRequestObject,
) (servedapi.RegisterResponseObject, error) {
	email, err := auth.ParseEmail(request.Body.Email)
	if err != nil {
		return servedapi.Register422JSONResponse{Body: a.validationError(ctx, bodyViolation("/email", "invalid", "Email is not a valid address"))}, nil
	}
	if err = auth.ValidatePassword(request.Body.Password); err != nil {
		return servedapi.Register422JSONResponse{Body: a.validationError(ctx, bodyViolation("/password", "invalid", "Password does not meet the policy"))}, nil
	}
	var user auth.User
	var issued sessions.Issued
	err = database.InTransaction(ctx, a.pool, func(txCtx context.Context) error {
		user, err = a.service.Register(txCtx, email, request.Body.Password)
		if err != nil {
			return err
		}
		issued, err = a.sessions.Establish(txCtx, user.ID)
		return err
	})
	if errors.Is(err, auth.ErrEmailTaken) {
		return servedapi.Register409JSONResponse{
			Body: apiErrorBody(ctx, codeEmailAlreadyRegistered, messageEmailAlreadyRegistered),
		}, nil
	}
	if err != nil {
		return servedapi.Register503JSONResponse{
			Body: apiErrorBody(ctx, codeServiceUnavailable, messageServiceUnavailable),
		}, nil
	}
	cookie := a.sessions.Cookie(issued)
	return servedapi.Register201JSONResponse{
		Body:    snapshotOf(user, issued.Snapshot),
		Headers: servedapi.Register201ResponseHeaders{SetCookie: &cookie},
	}, nil
}

func (a accounts) Login(
	ctx context.Context, request servedapi.LoginRequestObject,
) (servedapi.LoginResponseObject, error) {
	// An unusable email is answered as wrong credentials rather than as a validation failure, so a
	// caller cannot use the shape of the answer to tell a malformed address from an unknown one.
	email, err := auth.ParseEmail(request.Body.Email)
	if err != nil {
		return a.invalidCredentials(ctx), nil
	}
	var user auth.User
	var issued sessions.Issued
	err = database.InTransaction(ctx, a.pool, func(txCtx context.Context) error {
		user, err = a.service.Authenticate(txCtx, email, request.Body.Password)
		if err != nil {
			return err
		}
		// The session presented with this request, if any, is replaced rather than kept, so a
		// successful sign-in never leaves the previous account reachable from this browser.
		if err = a.sessions.Revoke(txCtx, sessionToken(ctx)); err != nil {
			return err
		}
		issued, err = a.sessions.Establish(txCtx, user.ID)
		return err
	})
	if errors.Is(err, auth.ErrInvalidCredentials) {
		return a.invalidCredentials(ctx), nil
	}
	if err != nil {
		return servedapi.Login503JSONResponse{
			Body: apiErrorBody(ctx, codeServiceUnavailable, messageServiceUnavailable),
		}, nil
	}
	cookie := a.sessions.Cookie(issued)
	return servedapi.Login200JSONResponse{
		Body:    snapshotOf(user, issued.Snapshot),
		Headers: servedapi.Login200ResponseHeaders{SetCookie: &cookie},
	}, nil
}

// Logout revokes exactly the session this browser presented. It answers the same way whether or
// not that session existed, so a client that never saw the first answer may safely retry.
func (a accounts) Logout(
	ctx context.Context, _ servedapi.LogoutRequestObject,
) (servedapi.LogoutResponseObject, error) {
	err := database.InTransaction(ctx, a.pool, func(txCtx context.Context) error {
		return a.sessions.Revoke(txCtx, sessionToken(ctx))
	})
	if err != nil {
		return servedapi.Logout503JSONResponse{
			Body: apiErrorBody(ctx, codeServiceUnavailable, messageServiceUnavailable),
		}, nil
	}
	cleared := a.sessions.ClearedCookie()
	return servedapi.Logout204Response{
		Headers: servedapi.Logout204ResponseHeaders{SetCookie: &cleared},
	}, nil
}

// GetMe restores the caller's context from the session cookie alone. No input selects the user, so
// a caller cannot read another account by supplying its identifier.
func (a accounts) GetMe(
	ctx context.Context, _ servedapi.GetMeRequestObject,
) (servedapi.GetMeResponseObject, error) {
	snapshot, live, err := sessionOf(ctx).resolve(ctx)
	if err != nil {
		return servedapi.GetMe503JSONResponse{
			Body: apiErrorBody(ctx, codeServiceUnavailable, messageServiceUnavailable),
		}, nil
	}
	if !live {
		return servedapi.GetMe401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}
	user, err := a.users.ByID(ctx, snapshot.UserID)
	if errors.Is(err, auth.ErrUserNotFound) {
		return servedapi.GetMe401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}
	if err != nil {
		return servedapi.GetMe503JSONResponse{
			Body: apiErrorBody(ctx, codeServiceUnavailable, messageServiceUnavailable),
		}, nil
	}
	return servedapi.GetMe200JSONResponse{Body: snapshotOf(user, snapshot)}, nil
}

func (a accounts) invalidCredentials(ctx context.Context) servedapi.Login401JSONResponse {
	return servedapi.Login401JSONResponse{
		Body: apiErrorBody(ctx, codeInvalidCredentials, messageInvalidCredentials),
	}
}

func (a accounts) validationError(ctx context.Context, failed violation) servedapi.ApiError {
	body := apiErrorBody(ctx, codeValidationFailed, messageValidationFailed)
	body.Details = violationDetails([]violation{failed})
	return body
}

// snapshotOf renders the session state the contract publishes. The session token is deliberately
// absent: it reaches the browser only as a cookie.
func snapshotOf(user auth.User, session sessions.Snapshot) servedapi.SessionSnapshot {
	return servedapi.SessionSnapshot{
		ServerTime:       timestamp(),
		CsrfToken:        session.CSRFToken,
		SessionExpiresAt: formatTimestamp(session.ExpiresAt),
		User: servedapi.User{
			Id:        user.ID.String(),
			Email:     openapi_types.Email(user.Email),
			CreatedAt: formatTimestamp(user.CreatedAt),
		},
	}
}
