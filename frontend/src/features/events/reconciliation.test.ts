import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { reconciliationEnabled } from './reconciliation.ts';

describe('the lever a browser check has over reconciliation', () => {
  test('reconciliation runs for every address but the one that switches it off', () => {
    assert.equal(reconciliationEnabled(''), true);
    assert.equal(reconciliationEnabled('?reconcile=off'), false);
    assert.equal(reconciliationEnabled('?reconcile=off&limit=1'), false);
  });

  // A value that is not the declared one leaves the shipped behaviour alone, so a mistyped address
  // cannot silently stop the reads that repair a missed signal.
  test('any other value leaves reconciliation running', () => {
    for (const search of ['?reconcile=', '?reconcile=on', '?reconcile=0', '?other=off']) {
      assert.equal(reconciliationEnabled(search), true, `search was ${search}`);
    }
  });
});
