import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { datedServiceMoment, serviceMoment } from './locale.ts';

describe('writing a moment of the contract', () => {
  // The contract states every moment as UTC, and the service states its days in Bishkek, which is
  // six hours ahead: a moment late in the UTC evening is the next morning where a person reads it.
  test('is read in the timezone the service states its days in', () => {
    assert.equal(serviceMoment('2026-09-12T20:30:00.000000Z'), '13 сентября в 02:30');
  });

  test('carries the year where a history needs it', () => {
    assert.equal(datedServiceMoment('2026-09-12T20:30:00.000000Z'), '13 сентября 2026 г. в 02:30');
  });

  test('a moment it cannot read is written as nothing rather than as a date', () => {
    assert.equal(serviceMoment('not a moment'), undefined);
    assert.equal(datedServiceMoment('not a moment'), undefined);
  });
});
