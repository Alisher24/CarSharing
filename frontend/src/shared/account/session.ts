import type { SessionSnapshot } from '../api/session.ts';
import type { SessionResult } from '../api/session.ts';

/**
 * What the interface knows about the signed-in person. It is held in memory only: the session lives
 * in an HttpOnly cookie the browser attaches, so nothing private is written to browser storage and a
 * new tab learns who it is by asking the server.
 *
 * It is declared above both the feature that owns the session and every feature that reads with it,
 * because a reader is keyed by the account rather than by a shape each of them rebuilds.
 */
export type Account =
  { state: 'checking' } | { state: 'signed-out' } | { state: 'signed-in'; snapshot: SessionSnapshot };

/**
 * The session one account is signed in under, or nothing when nobody is. It is what every private
 * reader is keyed by, so an answer read for one account is never shown to the next.
 */
export function sessionOf(account: Account): string | undefined {
  return account.state === 'signed-in' ? account.snapshot.user.id : undefined;
}

/** The token a command of one account must carry, or nothing when nobody is signed in. */
export function csrfTokenOf(account: Account): string | undefined {
  return account.state === 'signed-in' ? account.snapshot.csrf_token : undefined;
}

/** How a submitted form ended, so the entry screen can explain a refusal without guessing. */
export type Submission =
  | { state: 'idle' }
  | { state: 'sending' }
  | { state: 'failed'; result: Exclude<SessionResult, { outcome: 'session' }> };
