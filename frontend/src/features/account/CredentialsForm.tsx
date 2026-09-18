import { useRef, useState, type FormEvent, type RefObject } from 'react';
import { ENTRY_ACTIONS, ENTRY_INTENTS, ENTRY_TAB_TITLES, ENTRY_TABS_LABEL } from './accountCopy';
import { refusalText } from './refusalText';
import type { AccountIntent } from './accountIntent';
import type { Submission } from './useAccount';
import type { Credentials } from '../../shared/api/session';
import {
  PASSWORD_REQUIREMENT,
  emailRefusal,
  firstRefusal,
  passwordRefusal,
  type CredentialField,
} from './credentialRules';

/** The two fields, in the order the form reads them and the order they are checked in. */
const FIELDS: readonly CredentialField[] = ['email', 'password'];

const EMPTY_CREDENTIALS: Credentials = { email: '', password: '' };

/** Which fields have been checked once, which is what decides whether their reason is shown. */
type CheckedFields = Record<CredentialField, boolean>;

const UNCHECKED_FIELDS: CheckedFields = { email: false, password: false };
const EVERY_FIELD_CHECKED: CheckedFields = { email: true, password: true };

/** Everything about one field that does not depend on what is typed into it. */
type FieldFacts = { id: string; label: string; type: 'email' | 'password'; errorId: string; testId: string };

// The identifiers the fields and their explanations are addressed by. They are the ones the entry
// form has always published, so a check that drove it before drives the window that holds it now.
const FIELD_FACTS: Record<CredentialField, FieldFacts> = {
  email: {
    id: 'account-email-field',
    label: 'Электронная почта',
    type: 'email',
    errorId: 'account-email-error',
    testId: 'email-error',
  },
  password: {
    id: 'account-password-field',
    label: 'Пароль',
    type: 'password',
    errorId: 'account-password-error',
    testId: 'password-error',
  },
};

/** What a password manager is told a password is for, which differs between choosing and remembering. */
const PASSWORD_AUTOCOMPLETE: Record<AccountIntent, string> = {
  'sign-in': 'current-password',
  register: 'new-password',
};

type CredentialsFormProps = {
  /** Which operation the form is asking for, which is the tab the person chose. */
  intent: AccountIntent;
  submission: Submission;
  onSubmit: (intent: AccountIntent, credentials: Credentials) => Promise<void>;

  /** Which of the two operations the person chose, which decides the action and the window's title. */
  onChooseIntent: (intent: AccountIntent) => void;

  /** Drops the answer of the submission the person is no longer asking about. */
  onForgetRefusal: () => void;
};

/**
 * CredentialsForm is the one form of the entry window: the tab naming the operation, the two fields,
 * and the single action that operation is asked with.
 *
 * A field is checked when it is left and when the form is sent, and from then on while it is being
 * typed into: checking from the first character argues with somebody still typing, and checking only
 * on sending makes them send to learn the rules. What stops the form from being sent is the first
 * field that is wrong, which is where the cursor is put.
 *
 * None of this decides anything for the server. Every rule here is one the contract already states,
 * and a refusal the server answers with is shown exactly as it was.
 */
export function CredentialsForm({
  intent,
  submission,
  onSubmit,
  onChooseIntent,
  onForgetRefusal,
}: CredentialsFormProps) {
  const [credentials, setCredentials] = useState(EMPTY_CREDENTIALS);
  const [checked, setChecked] = useState(UNCHECKED_FIELDS);
  const emailInput = useRef<HTMLInputElement>(null);
  const passwordInput = useRef<HTMLInputElement>(null);
  const inputs = { email: emailInput, password: passwordInput };

  const sending = submission.state === 'sending';
  const failure = refusalText(submission);
  const shownErrors: Record<CredentialField, string | null> = {
    email: shownRefusal(checked.email, emailRefusal(credentials.email)),
    password: shownRefusal(checked.password, passwordRefusal(credentials.password)),
  };

  function send(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const stopped = firstRefusal(credentials);
    setChecked(EVERY_FIELD_CHECKED);

    if (stopped === null) {
      void onSubmit(intent, credentials);
      return;
    }

    inputs[stopped.field].current?.focus();
  }

  function change(field: CredentialField, value: string) {
    setCredentials((held) => ({ ...held, [field]: value }));
  }

  // A field is checked once the person leaves it, so a fault they have not finished typing is not
  // argued with — but from then on it is checked as it is corrected.
  function markChecked(field: CredentialField) {
    setChecked((held) => ({ ...held, [field]: true }));
  }

  // Choosing the tab that is already chosen is not a change, so nothing the person is reading is
  // taken from them by pressing it again.
  function choose(next: AccountIntent) {
    if (next === intent) return;

    setChecked(UNCHECKED_FIELDS);
    onChooseIntent(next);
    onForgetRefusal();
  }

  return (
    <form className="account-form" noValidate onSubmit={send}>
      <EntryTabs intent={intent} onChoose={choose} />
      {FIELDS.map((field) => (
        <CredentialInput
          key={field}
          field={field}
          value={credentials[field]}
          error={shownErrors[field]}
          hint={hintOf(field, intent)}
          autoComplete={autoCompleteOf(field, intent)}
          input={inputs[field]}
          onChange={(value) => change(field, value)}
          onBlur={() => markChecked(field)}
        />
      ))}
      {failure && (
        <p className="account-error" role="alert" data-testid="account-error">
          {failure}
        </p>
      )}
      <div className="account-actions">
        <button className="action-button" type="submit" disabled={sending}>
          {ENTRY_ACTIONS[intent]}
        </button>
      </div>
    </form>
  );
}

/** The two tabs, which are how the person says what they are doing before they do it. */
function EntryTabs({ intent, onChoose }: { intent: AccountIntent; onChoose: (intent: AccountIntent) => void }) {
  return (
    <div className="account-tabs" role="group" aria-label={ENTRY_TABS_LABEL}>
      {ENTRY_INTENTS.map((offered) => (
        <button
          key={offered}
          className="account-tab"
          type="button"
          aria-pressed={offered === intent}
          onClick={() => onChoose(offered)}
        >
          {ENTRY_TAB_TITLES[offered]}
        </button>
      ))}
    </div>
  );
}

type CredentialInputProps = {
  field: CredentialField;
  value: string;
  error: string | null;
  hint?: string;
  autoComplete: string;
  input: RefObject<HTMLInputElement | null>;
  onChange: (value: string) => void;
  onBlur: () => void;
};

/**
 * CredentialInput is one field with everything said about it: what it is called, the reason it is
 * refused and the requirement it is chosen under. The reason is a line of its own rather than a
 * colour, so what the field is waiting for is read by a person and by a screen reader alike.
 */
function CredentialInput({ field, value, error, hint, autoComplete, input, onChange, onBlur }: CredentialInputProps) {
  const facts = FIELD_FACTS[field];

  return (
    <>
      <label className="account-label" htmlFor={facts.id}>
        {facts.label}
      </label>
      <input
        className="account-field"
        id={facts.id}
        name={field}
        type={facts.type}
        autoComplete={autoComplete}
        required
        ref={input}
        value={value}
        aria-invalid={invalidFlag(error)}
        aria-describedby={explainedBy(error, facts.errorId)}
        onChange={(event) => onChange(event.target.value)}
        onBlur={onBlur}
      />
      {error && (
        <p className="account-field-error" id={facts.errorId} data-testid={facts.testId}>
          {error}
        </p>
      )}
      {hint && <p className="account-hint">{hint}</p>}
    </>
  );
}

/** The reason a field shows, which is the one it holds once it has been checked at least once. */
function shownRefusal(checked: boolean, refusal: string | null): string | null {
  return checked ? refusal : null;
}

/** What a field carries while it is being refused, and nothing at all while it is not. */
function invalidFlag(refusal: string | null): true | undefined {
  return refusal === null ? undefined : true;
}

/** What the field is described by while it is being refused: the line that says why. */
function explainedBy(refusal: string | null, errorId: string): string | undefined {
  return refusal === null ? undefined : errorId;
}

/**
 * What is said under a field before anything is wrong with it: the requirement a password being
 * chosen has to meet. A password being remembered was chosen long ago, so nothing is said there.
 */
function hintOf(field: CredentialField, intent: AccountIntent): string | undefined {
  if (field === 'email' || intent === 'sign-in') return undefined;

  return PASSWORD_REQUIREMENT;
}

/**
 * What a password manager is told a field is for. An address is an address either way; a password
 * being chosen is not the one being remembered, and saying so is what keeps the browser from
 * offering the saved one on the registration tab.
 */
function autoCompleteOf(field: CredentialField, intent: AccountIntent): string {
  if (field === 'email') return 'email';

  return PASSWORD_AUTOCOMPLETE[intent];
}
