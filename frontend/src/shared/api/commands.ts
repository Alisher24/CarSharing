import type { ApiError } from './generated/types.gen';

/**
 * What one attempt at a command of the service produced, whoever sent it and whatever it asked for.
 * The failures are kept apart because a person's next step differs: a refusal the server explained is
 * answered by reading the state again, a service that could not be reached leaves the outcome
 * unknown, and a session that has ended belongs to the account rather than to the command.
 */
export type CommandResult<T> =
  | { outcome: 'done'; answer: T; replayed: boolean }
  | { outcome: 'refused'; code: ApiError['code'] }
  | { outcome: 'unknown' }
  | { outcome: 'signed-out' };

/** What one command is sent with: the token of the session and the key of the attempt. */
export type CommandCredentials = { csrfToken: string; key: string };

// The session cookie is HttpOnly, so nothing here reads or writes it; the CSRF token comes from the
// session the caller holds in memory, and the browser attaches Origin itself.
export const sameOriginRequest = { credentials: 'same-origin', cache: 'no-store' } as const;

/** The origin the service demands of a command, which is the one this document is served from. */
export const originHeader = () => ({ Origin: window.location.origin });

/**
 * The headers every command carries: the origin it is sent from, the token of the session, and the
 * key that makes a repeat the same command rather than a second one.
 */
export function commandHeaders(credentials: CommandCredentials) {
  return { ...originHeader(), 'X-CSRF-Token': credentials.csrfToken, 'Idempotency-Key': credentials.key };
}

/**
 * answerOf turns one command call into its outcome. A transport failure is reported as an unknown
 * outcome rather than as a refusal: the command may have been carried out, and a client that called
 * it refused would tell a person their vehicle is free when it is not.
 */
export async function answerOf<T>(send: () => Promise<CommandResponse<T>>): Promise<CommandResult<T>> {
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

type CommandResponse<T> = { data?: T; error?: ApiError; response?: Response };

/** Whether the answer is the one an earlier attempt already gave. */
function replayedOf(response: Response | undefined): boolean {
  return response?.headers.get('Idempotency-Replayed') === 'true';
}
