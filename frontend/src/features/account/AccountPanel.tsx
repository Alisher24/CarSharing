import { useState, type FormEvent, type ReactNode } from 'react';
import type { AccountIntent } from './accountIntent';
import type { Submission } from './useAccount';
import { useAccount } from './useAccount';
import { refusalText } from './refusalText';
import type { Credentials, SessionSnapshot } from '../../shared/api/session';

// One section holds every account panel, so the heading it is labelled by is declared once.
const ACCOUNT_TITLE_ID = 'account-title';

export function AccountPanel() {
  const { account, submission, submit, leave } = useAccount();

  if (account.state === 'checking') {
    return (
      <AccountHeading>
        <h2 className="account-title" id={ACCOUNT_TITLE_ID}>
          Проверяем сессию…
        </h2>
      </AccountHeading>
    );
  }

  if (account.state === 'signed-in') {
    return <SignedInPanel snapshot={account.snapshot} submission={submission} onLeave={leave} />;
  }

  return <CredentialsForm submission={submission} onSubmit={submit} />;
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

function SignedInPanel({
  snapshot,
  submission,
  onLeave,
}: {
  snapshot: SessionSnapshot;
  submission: Submission;
  onLeave: () => Promise<void>;
}) {
  return (
    <AccountHeading>
      <h2 className="account-title" id={ACCOUNT_TITLE_ID}>
        Вы вошли
      </h2>
      <dl className="details">
        <div className="details-row">
          <dt className="details-term">Электронная почта</dt>
          <dd className="details-value" data-testid="account-email">
            {snapshot.user.email}
          </dd>
        </div>
      </dl>
      <button
        className="action-button"
        type="button"
        disabled={submission.state === 'sending'}
        onClick={() => void onLeave()}
      >
        Выйти
      </button>
    </AccountHeading>
  );
}

function CredentialsForm({
  submission,
  onSubmit,
}: {
  submission: Submission;
  onSubmit: (intent: AccountIntent, credentials: Credentials) => Promise<void>;
}) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const sending = submission.state === 'sending';
  const failure = refusalText(submission);

  function send(intent: AccountIntent) {
    void onSubmit(intent, { email, password });
  }

  // The form carries no default action: the person chooses between registering and signing in, so
  // the browser must not choose one for them. Each button states the operation it performs, so
  // which one was pressed is remembered nowhere between the press and the request.
  return (
    <AccountHeading>
      <h2 className="account-title" id={ACCOUNT_TITLE_ID}>
        Вход и регистрация
      </h2>
      <form className="account-form" onSubmit={preventDefault}>
        <label htmlFor="account-email-field">Электронная почта</label>
        <input
          className="account-field"
          id="account-email-field"
          name="email"
          type="email"
          autoComplete="email"
          required
          value={email}
          onChange={(event) => setEmail(event.target.value)}
        />
        <label htmlFor="account-password-field">Пароль</label>
        <input
          className="account-field"
          id="account-password-field"
          name="password"
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        />
        <div className="account-actions">
          <button className="action-button" type="button" disabled={sending} onClick={() => send('register')}>
            Зарегистрироваться
          </button>
          <button className="action-button" type="button" disabled={sending} onClick={() => send('sign-in')}>
            Войти
          </button>
        </div>
      </form>
      {failure && (
        <p className="account-error" role="alert" data-testid="account-error">
          {failure}
        </p>
      )}
    </AccountHeading>
  );
}

function preventDefault(event: FormEvent) {
  event.preventDefault();
}
