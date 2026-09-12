import { useState, type FormEvent, type ReactNode } from 'react';
import type { Submission } from './useAccount';
import { useAccount } from './useAccount';
import type { Credentials, SessionSnapshot } from '../../shared/api/session';

type Intent = 'register' | 'sign-in';

/**
 * Russian wording for each refusal the account operations can answer with, chosen by contract code
 * so that the text a person reads never depends on a message written for a developer.
 */
const REFUSAL_TEXT: Record<string, string> = {
  EMAIL_ALREADY_REGISTERED: 'Этот адрес уже зарегистрирован. Войдите в существующий аккаунт.',
  INVALID_CREDENTIALS: 'Неверный адрес или пароль.',
  VALIDATION_FAILED: 'Проверьте адрес электронной почты и пароль.',
  ORIGIN_NOT_ALLOWED: 'Запрос отклонён. Откройте приложение по обычному адресу.',
  SERVICE_UNAVAILABLE: 'Сервис временно недоступен. Повторите попытку позже.',
};

const UNREACHABLE_TEXT = 'Нет связи с сервисом. Проверьте подключение и повторите попытку.';

function refusalText(submission: Submission): string | null {
  if (submission.state !== 'failed') return null;
  if (submission.result.outcome === 'refused') {
    return REFUSAL_TEXT[submission.result.code] ?? UNREACHABLE_TEXT;
  }
  if (submission.result.outcome === 'signed-out') return REFUSAL_TEXT.INVALID_CREDENTIALS;

  return UNREACHABLE_TEXT;
}

export function AccountPanel() {
  const { account, submission, submit, leave } = useAccount();

  if (account.state === 'checking') {
    return (
      <AccountHeading>
        <h2 className="account-title" id="account-title">
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
    <section className="account" aria-labelledby="account-title">
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
      <h2 className="account-title" id="account-title">
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
  onSubmit: (intent: Intent, credentials: Credentials) => Promise<void>;
}) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [intent, setIntent] = useState<Intent>('register');
  const sending = submission.state === 'sending';
  const failure = refusalText(submission);

  function send(event: FormEvent) {
    event.preventDefault();
    void onSubmit(intent, { email, password });
  }

  return (
    <AccountHeading>
      <h2 className="account-title" id="account-title">
        Вход и регистрация
      </h2>
      <form className="account-form" onSubmit={send}>
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
          <button className="action-button" type="submit" disabled={sending} onClick={() => setIntent('register')}>
            Зарегистрироваться
          </button>
          <button className="action-button" type="submit" disabled={sending} onClick={() => setIntent('sign-in')}>
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
