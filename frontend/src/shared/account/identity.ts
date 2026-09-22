import { csrfTokenOf, sessionOf, type Account } from '../account/session.ts';

/**
 * What one account states about the caller: the account the private resources belong to, and the
 * token a command of that account must carry. Both are read from the session snapshot, so an account
 * that is not signed in has neither.
 *
 * A reader takes this whole value rather than reading the account itself, so the two are always read
 * from the same render: a session and a token that came from two readings are how a command is sent
 * with the credentials of a session that has already ended.
 */
export type SessionIdentity = { session: string | undefined; csrfToken: string | undefined };

/** What one account states about the caller, read once per render. */
export function identityOf(account: Account): SessionIdentity {
  return { session: sessionOf(account), csrfToken: csrfTokenOf(account) };
}
