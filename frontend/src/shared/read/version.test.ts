import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { compareVersions, highestVersion, isNewerTimestamp, isNewerVersion, NO_VERSION } from './version.ts';

describe('ordering two versions', () => {
  test('a longer decimal is the newer version', () => {
    assert.equal(compareVersions('42', '7'), 1);
    assert.equal(compareVersions('9', '10'), -1);
  });

  test('the same version is not newer than itself', () => {
    assert.equal(compareVersions('42', '42'), 0);
    assert.equal(isNewerVersion('42', '42'), false);
    assert.equal(highestVersion('42', '42'), '42');
  });

  // A version is a decimal string of up to nineteen digits, which is more than a JavaScript number
  // holds exactly: reading one as a number would make versions that differ compare as equal.
  test('versions beyond what a number holds exactly are still ordered', () => {
    assert.equal(isNewerVersion('9223372036854775807', '9223372036854775806'), true);
    assert.equal(isNewerVersion('9007199254740993', '9007199254740992'), true);
  });

  test('leading zeros do not make a version larger', () => {
    assert.equal(isNewerVersion('042', '7'), true);
    assert.equal(isNewerVersion('007', '42'), false);
  });

  test('the version that precedes every published one is smaller than any of them', () => {
    assert.equal(isNewerVersion('0', NO_VERSION), false);
    assert.equal(isNewerVersion('1', NO_VERSION), true);
  });
});

describe('ordering two snapshot moments', () => {
  test('a later moment is newer, and the same moment is not', () => {
    assert.equal(isNewerTimestamp('2026-09-12T07:15:30.123457Z', '2026-09-12T07:15:30.123456Z'), true);
    assert.equal(isNewerTimestamp('2026-09-12T07:15:30.123456Z', '2026-09-12T07:15:30.123456Z'), false);
    assert.equal(isNewerTimestamp('2026-09-12T07:15:30.123455Z', '2026-09-12T07:15:30.123456Z'), false);
  });
});
