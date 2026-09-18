import type { RefObject } from 'react';
import { PASSWORD_REQUIREMENT, type CredentialField } from './credentialRules';
import type { AccountIntent } from './accountIntent';

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

type CredentialInputProps = {
  field: CredentialField;

  /** Which operation the form is for, which decides what the field asks the password manager for. */
  intent: AccountIntent;
  value: string;

  /** Why the field is being refused, or nothing while it holds what the contract accepts. */
  error: string | null;
  input: RefObject<HTMLInputElement | null>;
  onChange: (value: string) => void;
  onBlur: () => void;
};

/**
 * CredentialInput is one field with everything said about it: what it is called, the reason it is
 * refused and the requirement it is chosen under. The reason is a line of its own rather than a
 * colour, so what the field is waiting for is read by a person and by a screen reader alike.
 */
export function CredentialInput({ field, intent, value, error, input, onChange, onBlur }: CredentialInputProps) {
  const facts = FIELD_FACTS[field];
  const hint = hintOf(field, intent);

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
        autoComplete={autoCompleteOf(field, intent)}
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
