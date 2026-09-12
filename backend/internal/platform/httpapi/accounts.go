package httpapi

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

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
	messageRateLimited            = "Too many attempts; try again later"
)

// Messages the account operations answer a malformed field with. Unlike the pair above, these name
// the field, because registration is where the field is chosen rather than guessed at.
const (
	messageEmailInvalid    = "Email is not a valid address"
	messagePasswordInvalid = "Password does not meet the policy"
)

// accounts answers the operations an account is created and proven through. Registration and
// sign-in each run as one transaction, so the user and the session that carries them reach storage
// together or not at all, and the cookie is written only once that transaction has committed.
type accounts struct {
	pool     *pgxpool.Pool
	sessions *sessions.Manager
	service  *auth.Service
	users    *auth.UserStore
	throttle *auth.Throttle
}

// newAccounts builds the account handlers, or names the dependency that is missing. Each of these
// is refused rather than defaulted, because a handler that reached a nil one would answer a request
// it never checked.
func newAccounts(dependencies Dependencies) (accounts, error) {
	if dependencies.Pool == nil {
		return accounts{}, fmt.Errorf("%w: database pool", ErrIncompleteApplication)
	}
	if dependencies.Sessions == nil {
		return accounts{}, fmt.Errorf("%w: session manager", ErrIncompleteApplication)
	}
	if dependencies.Auth == nil {
		return accounts{}, fmt.Errorf("%w: account service", ErrIncompleteApplication)
	}
	if dependencies.Users == nil {
		return accounts{}, fmt.Errorf("%w: user store", ErrIncompleteApplication)
	}
	if dependencies.Throttle == nil {
		return accounts{}, fmt.Errorf("%w: rate-limit throttle", ErrIncompleteApplication)
	}
	return accounts{
		pool:     dependencies.Pool,
		sessions: dependencies.Sessions,
		service:  dependencies.Auth,
		users:    dependencies.Users,
		throttle: dependencies.Throttle,
	}, nil
}

func (a accounts) Register(
	ctx context.Context, request servedapi.RegisterRequestObject,
) (servedapi.RegisterResponseObject, error) {
	email, refused, err := a.registrationPreflight(ctx, request)
	if refused != nil || err != nil {
		return refused, err
	}

	failure := a.recordRegistrationAttempt(ctx)
	if failure != nil {
		return *failure, nil
	}

	user, issued, err := a.registerAccount(ctx, email, request.Body.Password)
	if errors.Is(err, auth.ErrEmailTaken) {
		// The generated response type carries the 409 itself, so this code never reaches
		// writeError and has no transport alias.
		return servedapi.Register409JSONResponse{
			Body: apiErrorBody(ctx, servedapi.EMAILALREADYREGISTERED, messageEmailAlreadyRegistered),
		}, nil
	}

	if err != nil {
		return servedapi.Register503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}

	cookie := a.sessions.Cookie(issued)
	return servedapi.Register201JSONResponse{
		Body:    snapshotOf(user, issued.Snapshot),
		Headers: servedapi.Register201ResponseHeaders{SetCookie: &cookie},
	}, nil
}

// registrationPreflight parses the submitted fields and applies the address's rate limit. A
// non-nil response is the answer the request already has; a non-nil error is a failure of this
// server, which the caller reports rather than answering the request itself.
func (a accounts) registrationPreflight(
	ctx context.Context, request servedapi.RegisterRequestObject,
) (auth.Email, servedapi.RegisterResponseObject, error) {
	email, err := auth.ParseEmail(request.Body.Email)
	if err != nil {
		emailViolation := bodyViolation("/email", codeInvalidField, messageEmailInvalid)
		return "", servedapi.Register422JSONResponse{
			Body: a.validationError(ctx, emailViolation),
		}, nil
	}

	if err = auth.ValidatePassword(request.Body.Password); err != nil {
		passwordViolation := bodyViolation("/password", codeInvalidField, messagePasswordInvalid)
		return "", servedapi.Register422JSONResponse{
			Body: a.validationError(ctx, passwordViolation),
		}, nil
	}

	wait, allowed, err := a.throttle.RegistrationAllowed(ctx, clientAddress(ctx))
	if err != nil {
		return "", servedapi.Register503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}

	if allowed {
		return email, nil, nil
	}

	seconds := retryAfterSeconds(wait)
	return "", servedapi.Register429JSONResponse{
		Body:    apiErrorBody(ctx, codeRateLimited, messageRateLimited),
		Headers: servedapi.Register429ResponseHeaders{RetryAfter: &seconds},
	}, nil
}

// recordRegistrationAttempt spends one attempt of the requesting address's budget. The attempt is
// counted before it is carried out, so one that fails or is refused still spends the budget.
func (a accounts) recordRegistrationAttempt(ctx context.Context) *servedapi.RegisterResponseObject {
	if err := a.throttle.RecordRegistrationAttempt(ctx, clientAddress(ctx)); err != nil {
		refused := servedapi.RegisterResponseObject(servedapi.Register503JSONResponse{
			Body: serviceUnavailable(ctx),
		})
		return &refused
	}

	return nil
}

// registerAccount creates the account and the session that carries it in one transaction, so a
// failure at either step leaves neither behind.
func (a accounts) registerAccount(
	ctx context.Context, email auth.Email, password string,
) (auth.User, sessions.Issued, error) {
	var user auth.User
	var issued sessions.Issued
	err := database.InTransaction(ctx, a.pool, func(txCtx context.Context) error {
		var err error
		user, err = a.service.Register(txCtx, email, password)
		if err != nil {
			return err
		}
		issued, err = a.sessions.Establish(txCtx, user.ID)
		return err
	})
	return user, issued, err
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

	address := clientAddress(ctx)
	wait, allowed, err := a.throttle.SignInAllowed(ctx, email, address)
	if err != nil {
		return loginUnavailable(ctx), nil
	}

	// The limit is consulted before the password is verified, so a throttled attempt never pays
	// the memory-hard cost of a hash.
	if !allowed {
		seconds := retryAfterSeconds(wait)
		return servedapi.Login429JSONResponse{
			Body:    apiErrorBody(ctx, codeRateLimited, messageRateLimited),
			Headers: servedapi.Login429ResponseHeaders{RetryAfter: &seconds},
		}, nil
	}

	if err = a.throttle.RecordSignInAttempt(ctx, address); err != nil {
		return loginUnavailable(ctx), nil
	}

	user, issued, err := a.signIn(ctx, email, request.Body.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		return a.recordFailedSignIn(ctx, email, address)
	}
	if err != nil {
		return loginUnavailable(ctx), nil
	}

	cookie := a.sessions.Cookie(issued)
	return servedapi.Login200JSONResponse{
		Body:    snapshotOf(user, issued.Snapshot),
		Headers: servedapi.Login200ResponseHeaders{SetCookie: &cookie},
	}, nil
}

// signIn proves the credentials and replaces the session that carried the request with the one the
// proven account is given. Both run in one transaction, so a refused sign-in leaves the previous
// session exactly as it was.
func (a accounts) signIn(
	ctx context.Context, email auth.Email, password string,
) (auth.User, sessions.Issued, error) {
	var user auth.User
	var issued sessions.Issued
	err := database.InTransaction(ctx, a.pool, func(txCtx context.Context) error {
		var err error
		user, err = a.service.Authenticate(txCtx, email, password)
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
	return user, issued, err
}

// recordFailedSignIn answers a refused sign-in after counting it. The count is recorded outside the
// transaction the refused attempt just rolled back, which would otherwise undo the counter and
// leave the guess free; a counter that cannot be written is a service failure rather than a refusal,
// because the attempt was not charged for.
func (a accounts) recordFailedSignIn(
	ctx context.Context, email auth.Email, address string,
) (servedapi.LoginResponseObject, error) {
	if err := a.throttle.RecordSignInFailure(ctx, email, address); err != nil {
		return loginUnavailable(ctx), nil
	}
	return a.invalidCredentials(ctx), nil
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
		return servedapi.Logout503JSONResponse{Body: serviceUnavailable(ctx)}, nil
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
		return servedapi.GetMe503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	if !live {
		return authenticationRequired(ctx), nil
	}
	user, err := a.users.ByID(ctx, snapshot.UserID)
	if errors.Is(err, auth.ErrUserNotFound) {
		return authenticationRequired(ctx), nil
	}
	if err != nil {
		return servedapi.GetMe503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	return servedapi.GetMe200JSONResponse{Body: snapshotOf(user, snapshot)}, nil
}

// authenticationRequired answers a caller who is not signed in. It covers both a request without a
// live session and a session whose account is gone, because the caller's next step is the same.
func authenticationRequired(ctx context.Context) servedapi.GetMe401JSONResponse {
	return servedapi.GetMe401JSONResponse{
		Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
	}
}

func (a accounts) invalidCredentials(ctx context.Context) servedapi.Login401JSONResponse {
	return servedapi.Login401JSONResponse{
		Body: apiErrorBody(ctx, codeInvalidCredentials, messageInvalidCredentials),
	}
}

// validationError renders the contract's validation failure with the one field it reports, so the
// client learns which part of the payload the contract rejected.
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

// retryAfterSeconds renders a wait for the Retry-After header. Rounding up cannot produce the zero
// that would invite a caller straight back, because every wait arrives floored at
// ratelimit.MinimumRetryAfter.
func retryAfterSeconds(wait time.Duration) int {
	return int(math.Ceil(wait.Seconds()))
}
