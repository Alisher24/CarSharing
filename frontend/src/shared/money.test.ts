import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { INTERFACE_LOCALE } from './locale.ts';
import { SOM_UNIT, somText } from './money.ts';

/**
 * The acceptance check of money states the rule a second time on purpose: a browser check compares a
 * letter and a screen, and a check that read its expectation out of the interface would prove
 * nothing. What it must not do is state a different rule, so the two parts of the rule a check cannot
 * derive from the amount it is checking — the locale the grouping and the decimal mark come from, and
 * the unit an amount is written with — are declared once here and named by both.
 */
describe('the money an acceptance check compares against', () => {
  test('is written with the unit the interface declares', () => {
    assert.equal(SOM_UNIT, 'сома');
    assert.equal(somText('1234'), `12,34 ${SOM_UNIT}`);
  });

  test('is grouped and marked in the locale the interface declares', () => {
    const format = new Intl.NumberFormat(INTERFACE_LOCALE);
    // A hundred thousand som: read as a number it would lose the grouping the locale writes.
    assert.equal(somText('1234567890'), `${format.format(12_345_678)}${decimalMark(format)}90 ${SOM_UNIT}`);
  });
});

/** The decimal mark the interface locale writes, taken from the locale rather than assumed. */
function decimalMark(format: Intl.NumberFormat): string {
  const decimal = format.formatToParts(1.1).find((part) => part.type === 'decimal');
  assert.ok(decimal, 'the interface locale declares a decimal mark');

  return decimal.value;
}
