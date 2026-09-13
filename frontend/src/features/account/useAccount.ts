import { useCallback, useEffect, useState } from 'react';
import type { AccountIntent } from './accountIntent';
import { currentSession, registerAccount, signIn, signOut } from '../../shared/api/session';
import type { Credentials, SessionResult, SessionSnapshot } from '../../shared/api/session';

/**
 * Account is what the interface knows about the signed-in person. It is held in memory only: the
 * session lives in an HttpOnly cookie the browser attaches, so nothing private is written to
 * browser storage and a new tab learns who it is by asking the server.
 */
export type Account =
  { state: 'checking' } | { state: 'signed-out' } | { state: 'signed-in'; snapshot: SessionSnapshot };

/** How a submitted form ended, so the entry screen can explain a refusal without guessing. */
export type Submission =
  | { state: 'idle' }
  | { state: 'sending' }
  | { state: 'failed'; result: Exclude<SessionResult, { outcome: 'session' }> };

/** The operation each intent calls. A lookup rather than a branch, so a third intent is one entry. */
const SUBMIT_OPERATIONS: Record<AccountIntent, (credentials: Credentials) => Promise<SessionResult>> = {
  register: registerAccount,
  'sign-in': signIn,
};

/**
 * useAccount owns the session for the whole application rather than for the panel that shows it,
 * because the private stream is opened and closed by the session and must outlive any one screen.
 */
export function useAccount() {
  const [account, setAccount] = useState<Account>({ state: 'checking' });
  const [submission, setSubmission] = useState<Submission>({ state: 'idle' });
  const [check, setCheck] = useState(0);

  // The session is restored from the server on every load and after every reconnection, which is
  // what makes a reload keep the person signed in without the interface storing anything itself.
  useEffect(() => {
    const controller = new AbortController();
    let mounted = true;

    currentSession(controller.signal).then((result) => {
      if (mounted) setAccount((held) => accountAfterCheck(held, result));
    });

    return () => {
      mounted = false;
      controller.abort();
    };
  }, [check]);

  const recheck = useCallback(() => setCheck((count) => count + 1), []);

  const submit = useCallback(async (intent: AccountIntent, credentials: Credentials) => {
    setSubmission({ state: 'sending' });
    const result = await SUBMIT_OPERATIONS[intent](credentials);

    if (result.outcome !== 'session') {
      // A failed replacement leaves an account that is already signed in untouched, so only the
      // submission state changes here.
      setSubmission({ state: 'failed', result });
      return;
    }

    setSubmission({ state: 'idle' });
    setAccount({ state: 'signed-in', snapshot: result.snapshot });
  }, []);

  const leave = useCallback(async () => {
    if (account.state !== 'signed-in') return;

    setSubmission({ state: 'sending' });
    const revoked = await signOut(account.snapshot.csrf_token);
    setSubmission({ state: 'idle' });

    // The signed-out state is entered only once the server confirms the revocation, so clearing
    // the screen never claims a session was ended when it was not.
    if (revoked) setAccount({ state: 'signed-out' });
  }, [account]);

  return { account, submission, submit, leave, recheck };
}

/**
 * accountAfterCheck folds one answer of the session check into what is already known. A refusal is
 * the server saying there is no session; a check that could not be made says nothing about the
 * session at all, so a signed-in person stays signed in until the server itself says otherwise.
 */
function accountAfterCheck(held: Account, result: SessionResult): Account {
  if (result.outcome === 'session') return { state: 'signed-in', snapshot: result.snapshot };
  if (result.outcome === 'unreachable' && held.state === 'signed-in') return held;

  return { state: 'signed-out' };
}
