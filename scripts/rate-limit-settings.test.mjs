import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, test } from 'node:test';
import { rateLimitsFromEnvironment } from './acceptance/rate-limit-settings.mjs';

const configured = {
  RATE_LIMIT_SIGNIN_EMAIL_ADDRESS_ATTEMPTS: '10',
  RATE_LIMIT_SIGNIN_EMAIL_ATTEMPTS: '30',
  RATE_LIMIT_SIGNIN_ADDRESS_ATTEMPTS: '100',
  RATE_LIMIT_REGISTRATION_ADDRESS_ATTEMPTS: '10',
};
const ENVIRONMENT_EXAMPLE = readFileSync(new URL('../.env.example', import.meta.url), 'utf8');
const WORKFLOW = readFileSync(new URL('../.github/workflows/repository-checks.yml', import.meta.url), 'utf8');
const COMPOSE = readFileSync(new URL('../compose.yaml', import.meta.url), 'utf8');

describe('the limits an acceptance run expects', () => {
  test('are read from the environment passed to the service', () => {
    assert.deepEqual(rateLimitsFromEnvironment(configured), {
      signInEmailAndAddress: 10,
      signInEmail: 30,
      signInAddress: 100,
      registrationAddress: 10,
    });
  });

  test('refuse a run that did not export every limit', () => {
    const incomplete = { ...configured };
    delete incomplete.RATE_LIMIT_SIGNIN_EMAIL_ATTEMPTS;

    assert.throws(() => rateLimitsFromEnvironment(incomplete), /RATE_LIMIT_SIGNIN_EMAIL_ATTEMPTS is required/);
  });

  test('refuse a value that is not a positive integer', () => {
    assert.throws(
      () => rateLimitsFromEnvironment({ ...configured, RATE_LIMIT_SIGNIN_ADDRESS_ATTEMPTS: 'many' }),
      /RATE_LIMIT_SIGNIN_ADDRESS_ATTEMPTS must be a positive integer/,
    );
  });

  test('are exported with the same values by local setup and CI, then passed to the service', () => {
    for (const variable of Object.keys(configured)) {
      const local = ENVIRONMENT_EXAMPLE.match(new RegExp(`^${variable}=(\\d+)$`, 'm'));
      assert.ok(local, `.env.example does not export ${variable}`);

      const continuousIntegration = WORKFLOW.match(new RegExp(`^\\s+${variable}: "(\\d+)"$`, 'm'));
      assert.ok(continuousIntegration, `Compose integration does not export ${variable}`);
      assert.equal(continuousIntegration[1], local[1], `${variable} differs between local setup and CI`);

      const substitution = `${variable}: ` + '${' + `${variable}:-}`;
      assert.ok(COMPOSE.includes(substitution), `compose.yaml does not pass ${variable}`);
    }
  });
});
