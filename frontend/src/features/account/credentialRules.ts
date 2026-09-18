import type { Credentials } from '../../shared/api/session';

/**
 * The rules a credential must satisfy before it is worth sending to the server. They are the ones
 * the contract states, repeated in the interface so a person learns about a typo without a round
 * trip; the server checks every request again and stays the only authority on what it accepts.
 *
 * This module is the one declaration of those rules. The form asks it about a field and shows the
 * reason it answers with; a second copy beside the markup would disagree with this one the first
 * time the contract moved.
 */

/** The longest address the contract accepts, counted in code points. */
export const EMAIL_MAX_LENGTH = 254;

/** The range a password must fall in, counted in Unicode code points rather than UTF-16 units. */
export const PASSWORD_MIN_CODE_POINTS = 12;
export const PASSWORD_MAX_CODE_POINTS = 128;

/** Which of the two fields of the credentials a reason belongs to. */
export type CredentialField = 'email' | 'password';

/** The two fields in the order the form reads them and the order they are checked in. */
export const CREDENTIAL_FIELDS: readonly CredentialField[] = ['email', 'password'];

/** One field with the reason it cannot be sent, which is the field a form puts the cursor on. */
export type CredentialRefusal = { field: CredentialField; reason: string };

/** What a field with nothing in it is told, which is not the same as being told it is malformed. */
export const EMAIL_REQUIRED = 'Введите адрес электронной почты';
export const PASSWORD_REQUIRED = 'Введите пароль';

/** What each way of missing the contract is explained with, each naming the rule it broke. */
export const EMAIL_MALFORMED = 'Неверный формат адреса: нужен вид name@example.com';
export const EMAIL_TOO_LONG = `Адрес не длиннее ${EMAIL_MAX_LENGTH} символов`;
export const PASSWORD_LENGTH = `Пароль от ${PASSWORD_MIN_CODE_POINTS} до ${PASSWORD_MAX_CODE_POINTS} символов`;
export const PASSWORD_WHITESPACE = 'Пароль без пробелов';

/**
 * What the registration tab says under the password while it is being chosen, derived from the same
 * two bounds as the refusal above so a hint cannot promise a length the rule does not take.
 */
export const PASSWORD_REQUIREMENT = `От ${PASSWORD_MIN_CODE_POINTS} до ${PASSWORD_MAX_CODE_POINTS} символов, без пробелов`;

// The contract's pattern without the surrounding whitespace it tolerates: the address is checked in
// the canonical form the server stores, so padding a typo is not reported as one.
const EMAIL_SHAPE = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

// The blanks the contract enumerates in its own pattern for the password. A JavaScript `\s` is not
// that list — it holds the byte order mark and misses the next line character — so the class is
// written out rather than approximated.
const UNICODE_WHITESPACE = /[\t-\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]/;

/** The rule each field is held to, so the field order above decides the order of everything else. */
const FIELD_RULES: Record<CredentialField, (value: string) => string | null> = {
  email: emailRefusal,
  password: passwordRefusal,
};

/** Why an address cannot be sent, or nothing when it is one the contract accepts. */
export function emailRefusal(email: string): string | null {
  const canonical = email.trim();
  if (canonical === '') return EMAIL_REQUIRED;
  if (codePointLength(canonical) > EMAIL_MAX_LENGTH) return EMAIL_TOO_LONG;
  if (!EMAIL_SHAPE.test(canonical)) return EMAIL_MALFORMED;

  return null;
}

/** Why a password cannot be sent, or nothing when it is one the contract accepts. */
export function passwordRefusal(password: string): string | null {
  if (password === '') return PASSWORD_REQUIRED;

  const length = codePointLength(password);
  if (length < PASSWORD_MIN_CODE_POINTS || length > PASSWORD_MAX_CODE_POINTS) return PASSWORD_LENGTH;
  if (UNICODE_WHITESPACE.test(password)) return PASSWORD_WHITESPACE;

  return null;
}

/**
 * Which field stops these credentials from being sent, in the order the form reads them, so the
 * form can put the cursor on the first fault rather than making the person look for it.
 */
export function firstRefusal(credentials: Credentials): CredentialRefusal | null {
  for (const field of CREDENTIAL_FIELDS) {
    const reason = FIELD_RULES[field](credentials[field]);
    if (reason !== null) return { field, reason };
  }

  return null;
}

/**
 * How many code points a string holds. A password is measured this way because the contract
 * measures it this way: counting UTF-16 units would refuse a password the server accepts.
 */
function codePointLength(text: string): number {
  return [...text].length;
}
