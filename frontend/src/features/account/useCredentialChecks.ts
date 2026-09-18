import { useRef, useState, type RefObject } from 'react';
import {
  emailRefusal,
  firstRefusal,
  passwordRefusal,
  type CredentialField,
  type CredentialRefusal,
} from './credentialRules';
import type { Credentials } from '../../shared/api/session';

/** Which fields have been checked once, which is what decides whether their reason is shown. */
type CheckedFields = Record<CredentialField, boolean>;

const EMPTY_CREDENTIALS: Credentials = { email: '', password: '' };
const UNCHECKED_FIELDS: CheckedFields = { email: false, password: false };
const EVERY_FIELD_CHECKED: CheckedFields = { email: true, password: true };

/** What the entry form knows about the two fields it is checking. */
export type CredentialChecks = {
  /** What was typed, which is what the form sends once both fields hold a usable value. */
  values: Credentials;
  inputs: Record<CredentialField, RefObject<HTMLInputElement | null>>;
  errors: Record<CredentialField, string | null>;

  change: (field: CredentialField, value: string) => void;

  /** Records that the person left a field, from which on it is checked as it is typed into. */
  markChecked: (field: CredentialField) => void;

  /** Checks both fields, as sending does, and answers the first one that stops the form. */
  checkAll: () => CredentialRefusal | null;

  /** Drops what has been checked, which is what changing the tab does. */
  forgetChecks: () => void;

  /** Puts the cursor on one field, which is where the person who sent the form has to look. */
  focus: (field: CredentialField) => void;
};

/**
 * useCredentialChecks holds what the entry form knows about its two fields: what was typed, which of
 * them has been checked, and which reason is shown under each.
 *
 * A field is checked when it is left and when the form is sent, and from then on while it is being
 * typed into: checking from the first character argues with somebody still typing, and checking only
 * on sending makes them send to learn the rules. What the checks do not do is decide anything for
 * the server — every rule is one the contract already states.
 */
export function useCredentialChecks(): CredentialChecks {
  const [values, setValues] = useState(EMPTY_CREDENTIALS);
  const [checked, setChecked] = useState(UNCHECKED_FIELDS);
  const emailInput = useRef<HTMLInputElement>(null);
  const passwordInput = useRef<HTMLInputElement>(null);
  const inputs = { email: emailInput, password: passwordInput };

  return {
    values,
    inputs,
    errors: {
      email: shownRefusal(checked.email, emailRefusal(values.email)),
      password: shownRefusal(checked.password, passwordRefusal(values.password)),
    },
    change(field, value) {
      setValues((held) => ({ ...held, [field]: value }));
    },
    markChecked(field) {
      setChecked((held) => ({ ...held, [field]: true }));
    },
    // Sending checks every field, so a person who is refused reads all of what is wrong at once
    // rather than one fault per attempt.
    checkAll() {
      setChecked(EVERY_FIELD_CHECKED);
      return firstRefusal(values);
    },
    forgetChecks() {
      setChecked(UNCHECKED_FIELDS);
    },
    focus(field) {
      inputs[field].current?.focus();
    },
  };
}

/** The reason a field shows, which is the one it holds once it has been checked at least once. */
function shownRefusal(checked: boolean, refusal: string | null): string | null {
  return checked ? refusal : null;
}
