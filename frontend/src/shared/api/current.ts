import { cancelRental, getCurrentRental, reserve } from './generated/sdk.gen';
import type {
  ApiError,
  CurrentSnapshot,
  DailyLimitState,
  RentalCommandResult,
  ReserveResult,
} from './generated/types.gen';

export type {
  ApiError,
  CurrentRental,
  CurrentSnapshot,
  DailyLimitState,
  NoCurrentRental,
  Rental,
  RentalCommandResult,
  ReserveResult,
  TariffSnapshot,
} from './generated/types.gen';

/**
 * What one attempt at a reservation command produced. The three failures are kept apart because a
 * person's next step differs: a refusal the server explained is answered by reading the state again,
 * a service that could not be reached leaves the outcome unknown, and a session that has ended
 * belongs to the account rather than to the command.
 */
export type CommandResult<T> =
  | { outcome: 'done'; answer: T; replayed: boolean }
  | { outcome: 'refused'; code: ApiError['code'] }
  | { outcome: 'unknown' }
  | { outcome: 'signed-out' };

// The session cookie is HttpOnly, so nothing here reads or writes it; the CSRF token comes from the
// session the caller holds in memory, and the browser attaches Origin itself.
const sameOriginRequest = { credentials: 'same-origin', cache: 'no-store' } as const;

const originHeader = () => ({ Origin: window.location.origin });

/**
 * fetchCurrentRental reads what the account is doing now. It is a private resource: the browser
 * sends the session cookie, and the answer carries the current rental with the day's allowance.
 */
export async function fetchCurrentRental(signal: AbortSignal): Promise<CurrentSnapshot> {
  const { data } = await getCurrentRental({ ...sameOriginRequest, throwOnError: true, signal });
  return data;
}

/** What one reservation command is sent with: the token of the session and the key of the attempt. */
export type CommandCredentials = { csrfToken: string; key: string };

export async function reserveVehicle(
  vehicleId: string,
  { csrfToken, key }: CommandCredentials,
): Promise<CommandResult<ReserveResult>> {
  return answerOf(() =>
    reserve({
      body: { vehicle_id: vehicleId },
      headers: { ...originHeader(), 'X-CSRF-Token': csrfToken, 'Idempotency-Key': key },
      ...sameOriginRequest,
    }),
  );
}

export async function cancelReservation(
  rentalId: string,
  { csrfToken, key }: CommandCredentials,
): Promise<CommandResult<RentalCommandResult>> {
  return answerOf(() =>
    cancelRental({
      path: { id: rentalId },
      headers: { ...originHeader(), 'X-CSRF-Token': csrfToken, 'Idempotency-Key': key },
      ...sameOriginRequest,
    }),
  );
}

type CommandResponse<T> = { data?: T; error?: ApiError; response?: Response };

/**
 * answerOf turns one command call into its outcome. A transport failure is reported as an unknown
 * outcome rather than as a refusal: the command may have been carried out, and a client that called
 * it refused would tell a person their vehicle is free when it is not.
 */
async function answerOf<T>(send: () => Promise<CommandResponse<T>>): Promise<CommandResult<T>> {
  let response: CommandResponse<T>;
  try {
    response = await send();
  } catch {
    return { outcome: 'unknown' };
  }

  if (response.data !== undefined) {
    return { outcome: 'done', answer: response.data, replayed: replayedOf(response.response) };
  }
  if (response.response?.status === 401) return { outcome: 'signed-out' };
  if (response.error?.code) return { outcome: 'refused', code: response.error.code };

  return { outcome: 'unknown' };
}

/** Whether the answer is the one an earlier attempt already gave. */
function replayedOf(response: Response | undefined): boolean {
  return response?.headers.get('Idempotency-Replayed') === 'true';
}

/** The allowance of free reservations, as the current read publishes it. */
export function dailyLimitOf(snapshot: CurrentSnapshot): DailyLimitState {
  return snapshot.daily_limit;
}
