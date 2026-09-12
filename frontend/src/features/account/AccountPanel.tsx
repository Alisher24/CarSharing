import { useState, type FormEvent } from 'react';
import type { Submission } from './useAccount';
import { useAccount } from './useAccount';

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

function submissionText(submission: Submission): string | null {
  if (submission.state !== 'failed') return null;
  if (submission.result.outcome === 'refused') {
    return REFUSAL_TEXT[submission.result.code] ?? UNREACHABLE_TEXT;
  }
  if (submission.result.outcome === 'signed-out') return REFUSAL_TEXT.INVALID_CREDENTIALS;
  return UNREACHABLE_TEXT;
}

export function AccountPanel() {
  const { account, submission, submit, leave } = useAccount();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [intent, setIntent] = useState<'register' | 'sign-in'>('register');

  function send(event: FormEvent) {
    event.preventDefault();
    void submit(intent, { email, password });
  }

  if (account.state === 'checking') {
    return (
      <section className="account" aria-labelledby="account-title">
        <h2 id="account-title">Проверяем сессию…</h2>
      </section>
    );
  }

  if (account.state === 'signed-in') {
    return (
      <section className="account" aria-labelledby="account-title">
        <span className="section-label">УЧЁТНАЯ ЗАПИСЬ</span>
        <h2 id="account-title">Вы вошли</h2>
        <dl>
          <div>
            <dt>Электронная почта</dt>
            <dd data-testid="account-email">{account.snapshot.user.email}</dd>
          </div>
        </dl>
        <button type="button" disabled={submission.state === 'sending'} onClick={() => void leave()}>
          Выйти
        </button>
      </section>
    );
  }

  const failure = submissionText(submission);

  return (
    <section className="account" aria-labelledby="account-title">
      <span className="section-label">УЧЁТНАЯ ЗАПИСЬ</span>
      <h2 id="account-title">Вход и регистрация</h2>
      <form onSubmit={send}>
        <label htmlFor="account-email-field">Электронная почта</label>
        <input
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
          id="account-password-field"
          name="password"
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        />
        <div className="account-actions">
          <button type="submit" disabled={submission.state === 'sending'} onClick={() => setIntent('register')}>
            Зарегистрироваться
          </button>
          <button type="submit" disabled={submission.state === 'sending'} onClick={() => setIntent('sign-in')}>
            Войти
          </button>
        </div>
      </form>
      {failure === null ? null : (
        <p className="account-error" role="alert" data-testid="account-error">
          {failure}
        </p>
      )}
    </section>
  );
}
