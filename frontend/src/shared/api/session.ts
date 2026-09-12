import { getMe, login, logout, register } from './generated/sdk.gen';
import type { ApiError, SessionSnapshot } from './generated/types.gen';

export type { SessionSnapshot } from './generated/types.gen';

/**
 * What a call that establishes or reads a session produced. A refusal the server explained is
 * carried as its contract code, because the interface chooses its Russian wording from that code
 * rather than from a message the server wrote for a developer.
 */
export type SessionResult =
  | { outcome: 'session'; snapshot: SessionSnapshot }
  | { outcome: 'signed-out' }
  | { outcome: 'refused'; code: ApiError['code'] }
  | { outcome: 'unreachable' };

/** Credentials as a person typed them. The server canonicalizes the email and answers with it. */
export type Credentials = { email: string; password: string };

// The session cookie is HttpOnly, so nothing here reads or writes it: the browser attaches it and
// the server replaces it. The snapshot is held in memory by the caller and never in storage.
const sameOriginRequest = { credentials: 'same-origin', cache: 'no-store' } as const;

// The contract declares Origin as a required header, so the generated types ask for it. A browser
// sets Origin itself and refuses to let a script override it, so this value satisfies the type
// while the value the server actually checks is the one the browser attached.
const browserOrigin = () => ({ Origin: window.location.origin });

export async function registerAccount(credentials: Credentials): Promise<SessionResult> {
  return establish(() => register({ body: credentials, headers: browserOrigin(), ...sameOriginRequest }));
}

export async function signIn(credentials: Credentials): Promise<SessionResult> {
  return establish(() => login({ body: credentials, headers: browserOrigin(), ...sameOriginRequest }));
}

/**
 * currentSession asks the server who the caller is now. It is the only authority on that question:
 * a lost response, a message from another tab and a returning tab are all settled by asking again.
 */
export async function currentSession(signal?: AbortSignal): Promise<SessionResult> {
  return establish(() => getMe({ signal, ...sameOriginRequest }));
}

/**
 * signOut revokes this browser's session and reports whether the server confirmed it. Clearing the
 * screen revokes nothing, so an unconfirmed sign-out is reported as such rather than assumed. The
 * CSRF token comes from the session being ended, which is the only session it authorizes.
 */
export async function signOut(csrfToken: string): Promise<boolean> {
  try {
    const answer = await logout({
      headers: { ...browserOrigin(), 'X-CSRF-Token': csrfToken },
      ...sameOriginRequest,
    });
    return answer.response?.status === 204;
  } catch {
    return false;
  }
}

type Answer = { data?: SessionSnapshot; error?: ApiError; response?: Response };

/**
 * establish turns one contract call into a session result. A transport failure is reported as
 * unreachable rather than as a refusal, because the two lead a person to different next steps.
 */
async function establish(call: () => Promise<Answer>): Promise<SessionResult> {
  let answer: Answer;
  try {
    answer = await call();
  } catch {
    return { outcome: 'unreachable' };
  }
  if (answer.data) {
    return { outcome: 'session', snapshot: answer.data };
  }
  if (answer.response?.status === 401) {
    return { outcome: 'signed-out' };
  }
  if (answer.error?.code) {
    return { outcome: 'refused', code: answer.error.code };
  }
  return { outcome: 'unreachable' };
}
