import { useCallback, useEffect, useState } from 'react';
import {
  currentSession,
  registerAccount,
  signIn,
  signOut,
  type Credentials,
  type SessionResult,
  type SessionSnapshot,
} from '../../shared/api/session';

/**
 * Account is what the interface knows about the signed-in person. It is held in memory only: the
 * session lives in an HttpOnly cookie the browser attaches, so nothing private is written to
 * browser storage and a new tab learns who it is by asking the server.
 */
export type Account =
  | { state: 'checking' }
  | { state: 'signed-out' }
  | { state: 'signed-in'; snapshot: SessionSnapshot };

/** How a submitted form ended, so the entry screen can explain a refusal without guessing. */
export type Submission =
  | { state: 'idle' }
  | { state: 'sending' }
  | { state: 'failed'; result: Exclude<SessionResult, { outcome: 'session' }> };

export function useAccount() {
  const [account, setAccount] = useState<Account>({ state: 'checking' });
  const [submission, setSubmission] = useState<Submission>({ state: 'idle' });

  // The session is restored from the server on every load, which is what makes a reload keep the
  // person signed in without the interface storing anything itself.
  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    currentSession(controller.signal).then((result) => {
      if (!active) return;
      setAccount(result.outcome === 'session' ? { state: 'signed-in', snapshot: result.snapshot } : { state: 'signed-out' });
    });
    return () => {
      active = false;
      controller.abort();
    };
  }, []);

  const submit = useCallback(
    async (intent: 'register' | 'sign-in', credentials: Credentials) => {
      setSubmission({ state: 'sending' });
      const result = await (intent === 'register' ? registerAccount(credentials) : signIn(credentials));
      if (result.outcome === 'session') {
        setSubmission({ state: 'idle' });
        setAccount({ state: 'signed-in', snapshot: result.snapshot });
        return;
      }
      // A failed replacement leaves the account that is already signed in untouched, so only the
      // submission state changes here.
      setSubmission({ state: 'failed', result });
    },
    [],
  );

  const leave = useCallback(async () => {
    if (account.state !== 'signed-in') return;
    setSubmission({ state: 'sending' });
    const revoked = await signOut(account.snapshot.csrf_token);
    setSubmission({ state: 'idle' });
    // The signed-out state is entered only once the server confirms the revocation, so clearing
    // the screen never claims a session was ended when it was not.
    if (revoked) setAccount({ state: 'signed-out' });
  }, [account]);

  return { account, submission, submit, leave };
}
