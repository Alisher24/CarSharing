// The locale and the unit an amount is written with, declared in two places that cannot import one
// another: the interface writes every price through `frontend/src/shared/money.ts`, and the HTTP
// acceptance suites restate the rule so that a check of an invoice compares against something other
// than the interface it is checking. The restatement is deliberate; a restatement of a *different*
// rule is not, which is what this holds together.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, test } from 'node:test';

const MONEY = readFileSync(new URL('../frontend/src/shared/money.ts', import.meta.url), 'utf8');
const ACCEPTANCE_MONEY = readFileSync(new URL('./acceptance/money.mjs', import.meta.url), 'utf8');

/** The locale the interface writes numbers in, as `frontend/src/shared/locale.ts` declares it. */
function interfaceLocale() {
  const declared = readFileSync(new URL('../frontend/src/shared/locale.ts', import.meta.url), 'utf8').match(
    /INTERFACE_LOCALE\s*=\s*'([^']+)'/,
  );
  assert.ok(declared, 'the interface declares no locale');

  return declared[1];
}

/** One `const NAME = 'value'` declaration of a module, as that module writes it. */
function declaredText(source, name) {
  const declared = source.match(new RegExp(`(?:const|export const)\\s+${name}\\s*=\\s*'([^']*)'`));
  assert.ok(declared, `${name} is not declared as a text`);

  return declared[1];
}

describe('the money an acceptance check compares against', () => {
  test('is written with the unit the interface declares', () => {
    assert.equal(declaredText(ACCEPTANCE_MONEY, 'SOM_UNIT'), declaredText(MONEY, 'SOM_UNIT'));
  });

  test('is written in the locale the interface declares', () => {
    assert.equal(declaredText(ACCEPTANCE_MONEY, 'INTERFACE_LOCALE'), interfaceLocale());
    assert.match(MONEY, /new Intl\.NumberFormat\(INTERFACE_LOCALE\)/);
  });
});
