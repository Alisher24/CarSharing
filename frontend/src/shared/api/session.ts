import { getMe, login, logout, register } from './generated/sdk.gen';
import type { ApiError, SessionSnapshot } from './generated/types.gen';
import { originHeader, sameOriginRequest } from './request.ts';

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

type SessionResponse = { data?: SessionSnapshot; error?: ApiError; response?: Response };

export async function registerAccount(credentials: Credentials): Promise<SessionResult> {
  return toSessionResult(() => register({ body: credentials, headers: originHeader(), ...sameOriginRequest }));
}

export async function signIn(credentials: Credentials): Promise<SessionResult> {
  return toSessionResult(() => login({ body: credentials, headers: originHeader(), ...sameOriginRequest }));
}

/**
 * currentSession asks the server who the caller is now. It is the only authority on that question:
 * a lost response, a message from another tab and a returning tab are all settled by asking again.
 */
export async function currentSession(signal?: AbortSignal): Promise<SessionResult> {
  return toSessionResult(() => getMe({ signal, ...sameOriginRequest }));
}

/**
 * signOut revokes this browser's session and reports whether the server confirmed it. Clearing the
 * screen revokes nothing, so an unconfirmed sign-out is reported as such rather than assumed. The
 * CSRF token comes from the session being ended, which is the only session it authorizes.
 */
export async function signOut(csrfToken: string): Promise<boolean> {
  try {
    const response = await logout({
      headers: { ...originHeader(), 'X-CSRF-Token': csrfToken },
      ...sameOriginRequest,
    });

    return response.response?.status === 204;
  } catch {
    return false;
  }
}

/**
 * toSessionResult turns one contract call into a session result. A transport failure is reported as
 * unreachable rather than as a refusal, because the two lead a person to different next steps.
 */
async function toSessionResult(call: () => Promise<SessionResponse>): Promise<SessionResult> {
  let response: SessionResponse;
  try {
    response = await call();
  } catch {
    return { outcome: 'unreachable' };
  }

  if (response.data) return { outcome: 'session', snapshot: response.data };

  // Only the session endpoints answer 401, and they do it for one reason: the request carried no
  // live session. Reading it as anything else would show a refusal where there is simply no one
  // signed in.
  if (response.response?.status === 401) return { outcome: 'signed-out' };
  if (response.error?.code) return { outcome: 'refused', code: response.error.code };

  return { outcome: 'unreachable' };
}
