import type { ReactNode } from 'react';
import { CredentialsForm } from './CredentialsForm';
import type { Account, Submission } from './useAccount';
import type { AccountIntent } from './accountIntent';
import type { Credentials } from '../../shared/api/session';

// One section holds every account panel, so the heading it is labelled by is declared once.
const ACCOUNT_TITLE_ID = 'account-title';

type AccountPanelProps = {
  account: Account;
  submission: Submission;
  onSubmit: (intent: AccountIntent, credentials: Credentials) => Promise<void>;
};

/**
 * AccountPanel is the way in from the map: the entry form, and the statement that the session is
 * still being checked. It holds nothing of the session, which is restored, replaced and ended above
 * it, so closing the panel changes nothing about who is signed in or about the stream that belongs
 * to them.
 *
 * What a signed-in person reads about their account — the address they are signed in as, and the
 * control that ends the session — is the cabinet's, which is where the header leads once there is a
 * session to read.
 */
export function AccountPanel({ account, submission, onSubmit }: AccountPanelProps) {
  if (account.state === 'checking') {
    return (
      <AccountHeading>
        <h2 className="account-title" id={ACCOUNT_TITLE_ID}>
          Проверяем сессию…
        </h2>
      </AccountHeading>
    );
  }

  return (
    <AccountHeading>
      <h2 className="account-title" id={ACCOUNT_TITLE_ID}>
        Вход и регистрация
      </h2>
      <CredentialsForm submission={submission} onSubmit={onSubmit} />
    </AccountHeading>
  );
}

/** Every panel renders the same section and label, so only the panel itself decides the rest. */
function AccountHeading({ children }: { children: ReactNode }) {
  return (
    <section className="account" aria-labelledby={ACCOUNT_TITLE_ID}>
      <span className="section-label">УЧЁТНАЯ ЗАПИСЬ</span>
      {children}
    </section>
  );
}
