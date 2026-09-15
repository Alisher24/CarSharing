import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { INVOICES_ADDRESS, invoiceAddress, MAP_ADDRESS, RIDES_ADDRESS, sectionOf } from './addresses.ts';

describe('the address of one invoice', () => {
  test('names the invoice inside the feed it belongs to', () => {
    assert.equal(
      invoiceAddress('01994342-6ba7-7000-8000-000000000001'),
      `${INVOICES_ADDRESS}/01994342-6ba7-7000-8000-000000000001`,
    );
  });

  // An identifier is put in an address by this function alone, so whatever a stored record carries
  // becomes one path segment rather than a second segment or a query of its own.
  test('carries an identifier that is not one segment as one segment', () => {
    assert.equal(invoiceAddress('a/b?c'), `${INVOICES_ADDRESS}/a%2Fb%3Fc`);
  });
});

describe('which feed of the cabinet an address is in', () => {
  test('the feed of the rides', () => {
    assert.equal(sectionOf(RIDES_ADDRESS), 'rides');
  });

  test('the feed of the invoices', () => {
    assert.equal(sectionOf(INVOICES_ADDRESS), 'invoices');
  });

  // One invoice is read through the feed of the invoices, so the tab a person came through stays
  // marked as the current one while they are reading it.
  test('one invoice is in the feed of the invoices', () => {
    assert.equal(sectionOf(invoiceAddress('invoice-1')), 'invoices');
  });

  test('an address outside the cabinet is in no feed of it', () => {
    assert.equal(sectionOf(MAP_ADDRESS), undefined);
    assert.equal(sectionOf('/account'), undefined);
    assert.equal(sectionOf('/account/rides/extra'), undefined);
    assert.equal(sectionOf('/account/invoicesomething'), undefined);
  });
});
