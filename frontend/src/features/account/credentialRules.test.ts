import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, test } from 'node:test';
import {
  EMAIL_MAX_LENGTH,
  EMAIL_MALFORMED,
  EMAIL_REQUIRED,
  EMAIL_TOO_LONG,
  PASSWORD_LENGTH,
  PASSWORD_MAX_CODE_POINTS,
  PASSWORD_MIN_CODE_POINTS,
  PASSWORD_REQUIRED,
  PASSWORD_WHITESPACE,
  emailRefusal,
  firstRefusal,
  passwordRefusal,
} from './credentialRules.ts';

/** The address every check needs a good one of, so the field under test is the only difference. */
const VALID_EMAIL = 'someone@example.test';

/** The password every check needs a good one of, for the same reason. */
const VALID_PASSWORD = 'correcthorsebattery';

describe('the rule an address must satisfy', () => {
  test('refuses an address with no at sign', () => {
    assert.equal(emailRefusal('someone.example.test'), EMAIL_MALFORMED);
  });

  test('refuses an address whose domain has no dot', () => {
    assert.equal(emailRefusal('someone@example'), EMAIL_MALFORMED);
  });

  test('refuses an address with whitespace inside it', () => {
    assert.equal(emailRefusal('some one@example.test'), EMAIL_MALFORMED);
  });

  test('accepts an address the server would trim itself', () => {
    assert.equal(emailRefusal(`  ${VALID_EMAIL}  `), null);
  });

  test('accepts the longest address the contract allows and refuses one character more', () => {
    assert.equal(emailRefusal(addressOfLength(EMAIL_MAX_LENGTH)), null);
    assert.equal(emailRefusal(addressOfLength(EMAIL_MAX_LENGTH + 1)), EMAIL_TOO_LONG);
  });

  test('asks for an address rather than reporting its format when nothing was typed', () => {
    assert.equal(emailRefusal(''), EMAIL_REQUIRED);
    assert.equal(emailRefusal('   '), EMAIL_REQUIRED);
  });
});

describe('the rule a password must satisfy', () => {
  test('refuses eleven code points and accepts twelve', () => {
    assert.equal(passwordRefusal('a'.repeat(PASSWORD_MIN_CODE_POINTS - 1)), PASSWORD_LENGTH);
    assert.equal(passwordRefusal('a'.repeat(PASSWORD_MIN_CODE_POINTS)), null);
  });

  test('accepts the longest password the contract allows and refuses one code point more', () => {
    assert.equal(passwordRefusal('a'.repeat(PASSWORD_MAX_CODE_POINTS)), null);
    assert.equal(passwordRefusal('a'.repeat(PASSWORD_MAX_CODE_POINTS + 1)), PASSWORD_LENGTH);
  });

  test('counts code points rather than the units a JavaScript string holds', () => {
    const elevenLettersAndOneAstralCharacter = 'a'.repeat(11) + '😀';

    assert.equal(elevenLettersAndOneAstralCharacter.length, 13);
    assert.equal(passwordRefusal(elevenLettersAndOneAstralCharacter), null);
  });

  test('refuses every blank the contract enumerates, not only the space bar', () => {
    const blanks = [' ', '\t', '\n', '\u00a0', '\u2003', '\u3000'];

    for (const blank of blanks) {
      assert.equal(passwordRefusal('a'.repeat(6) + blank + 'a'.repeat(6)), PASSWORD_WHITESPACE, blank);
    }
  });

  test('asks for a password rather than reporting its length when nothing was typed', () => {
    assert.equal(passwordRefusal(''), PASSWORD_REQUIRED);
  });
});

describe('the first field that stops the credentials from being sent', () => {
  test('is the address, which is the field the form reads first', () => {
    const refusal = firstRefusal({ email: 'someone.example.test', password: 'short' });

    assert.deepEqual(refusal, { field: 'email', reason: EMAIL_MALFORMED });
  });

  test('is the password once the address is usable', () => {
    const refusal = firstRefusal({ email: VALID_EMAIL, password: 'short' });

    assert.deepEqual(refusal, { field: 'password', reason: PASSWORD_LENGTH });
  });

  test('is nothing at all when both fields hold what the contract accepts', () => {
    assert.equal(firstRefusal({ email: VALID_EMAIL, password: VALID_PASSWORD }), null);
  });
});

describe('the limits the interface repeats from the contract', () => {
  test('are the ones the Credentials schema declares', () => {
    const credentials = schemaBlock('Credentials');
    const email = fieldBlock(credentials, 'email');
    const password = fieldBlock(credentials, 'password');

    assert.equal(EMAIL_MAX_LENGTH, declaredLength(email, 'maxLength'));
    assert.equal(PASSWORD_MIN_CODE_POINTS, declaredLength(password, 'minLength'));
    assert.equal(PASSWORD_MAX_CODE_POINTS, declaredLength(password, 'maxLength'));
  });
});

/** An address of exactly this many characters, which is the field the length rules are read on. */
function addressOfLength(length: number): string {
  const suffix = '@example.test';
  return `${'a'.repeat(length - suffix.length)}${suffix}`;
}

// The contract is read as text: the interface carries no YAML reader, and the three numbers it
// repeats are what a check has to find and compare.
const CONTRACT = readFileSync(new URL('../../../../openapi/public.yaml', import.meta.url), 'utf8');

/** The text one schema of the contract occupies, from its declaration to the next one. */
function schemaBlock(name: string): string {
  return blockOf(CONTRACT, name, '    ');
}

/** The text one field of a schema occupies, from its declaration to the next one. */
function fieldBlock(schema: string, field: string): string {
  return blockOf(schema, field, '        ');
}

function blockOf(text: string, declaration: string, indent: string): string {
  const marker = `\n${indent}${declaration}:\n`;
  const start = text.indexOf(marker);
  assert.notEqual(start, -1, `the contract declares no ${declaration} here`);

  const rest = text.slice(start + marker.length);
  const end = rest.search(new RegExp(`\\n${indent}\\S`));

  return end === -1 ? rest : rest.slice(0, end);
}

/** One length bound a schema states, which is the number this module must not drift from. */
function declaredLength(block: string, bound: string): number {
  const found = new RegExp(`\\n\\s+${bound}: (\\d+)`).exec(block);
  assert.ok(found, `the contract declares no ${bound} here`);

  return Number(found[1]);
}
