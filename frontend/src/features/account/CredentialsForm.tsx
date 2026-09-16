import { useState, type FormEvent } from 'react';
import type { AccountIntent } from './accountIntent';
import { refusalText } from './refusalText';
import type { Submission } from './useAccount';
import type { Credentials } from '../../shared/api/session';

type CredentialsFormProps = {
  submission: Submission;
  onSubmit: (intent: AccountIntent, credentials: Credentials) => Promise<void>;
};

/**
 * CredentialsForm is the one way into the application: the panel above the map shows it, and so does
 * the cabinet when it is opened without a session. It is one component rather than two, so a person
 * who follows a link to their own invoice reads the same form and stays at the address they were
 * going to.
 *
 * The form carries no default action: the person chooses between registering and signing in, so the
 * browser must not choose one for them. Each button states the operation it performs, so which one
 * was pressed is remembered nowhere between the press and the request.
 */
export function CredentialsForm({ submission, onSubmit }: CredentialsFormProps) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const sending = submission.state === 'sending';
  const failure = refusalText(submission);

  function send(intent: AccountIntent) {
    void onSubmit(intent, { email, password });
  }

  return (
    <>
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
    </>
  );
}

function preventDefault(event: FormEvent) {
  event.preventDefault();
}
