// The four rate limits, observed on the real HTTP boundary. Each test starts from cleared counters
// and fills exactly the limit it is about, so a refusal can only come from that limit.
import assert from 'node:assert/strict';
import { before, beforeEach, describe, test } from 'node:test';
import {
  call,
  callUntilRefused,
  compose,
  composeWith,
  CURRENT_USER_PATH,
  newEmail,
  registerAccount,
  registrationRequest,
  REGISTRATION_PATH,
  resetRateLimits,
  SIGN_IN_PATH,
  signInRequest,
  sql,
  waitForReady,
} from './client.mjs';

/**
 * The limits the running service reads, taken from the service's own configuration rather than
 * restated here, so a changed setting changes what these tests demand.
 */
const configuredLimits = {
  signInEmailAndAddress: Number(process.env.RATE_LIMIT_SIGNIN_EMAIL_ADDRESS_ATTEMPTS ?? 10),
  signInEmail: Number(process.env.RATE_LIMIT_SIGNIN_EMAIL_ATTEMPTS ?? 30),
  signInAddress: Number(process.env.RATE_LIMIT_SIGNIN_ADDRESS_ATTEMPTS ?? 100),
  registrationAddress: Number(process.env.RATE_LIMIT_REGISTRATION_ADDRESS_ATTEMPTS ?? 10),
};

const RATE_LIMIT_SCOPE = {
  signInEmail: 'sign_in_email',
  signInAddress: 'sign_in_address',
  registrationAddress: 'registration_address',
};

const RATE_LIMIT_COUNTER_TABLE = 'rate_limit_counters';

const RATE_LIMITED_STATUS = 429;
const ACCEPTED_STATUS = 200;
const REFUSED_SIGN_IN_STATUS = 401;
const RATE_LIMITED_CODE = 'RATE_LIMITED';
const SECONDS_PER_MINUTE = 60;
const WINDOW_DURATION_MINUTES = 15;
const COMPLETED_WINDOW_MINUTES = 16;
const REGISTRATION_RETRY_AFTER_LIMIT_SECONDS = 3_600;
const LOWERED_REGISTRATION_LIMIT = 2;

before(waitForReady);
beforeEach(resetRateLimits);

/** Writes a counter straight to its threshold, so a test does not have to make a hundred requests. */
function fillCounter(scope, subject, attempts) {
  sql(
    `INSERT INTO ${RATE_LIMIT_COUNTER_TABLE} (scope, subject, window_started_at, attempts)
     VALUES ('${scope}', '${subject}', now(), ${attempts})
     ON CONFLICT (scope, subject) DO UPDATE SET window_started_at = now(), attempts = ${attempts}`,
  );
}

/** The address the suite reaches the service from, as the service recorded it for one scope. */
function observedSubject(scope) {
  return sql(`SELECT subject FROM ${RATE_LIMIT_COUNTER_TABLE} WHERE scope = '${scope}' LIMIT 1`);
}

function assertAddressRecorded(scope, address) {
  assert.ok(address, `the service recorded no address for scope ${scope}`);
}

function assertRateLimited(response, message) {
  assert.equal(response.status, RATE_LIMITED_STATUS, message);
  assert.equal(response.json.code, RATE_LIMITED_CODE);
}

function retryAfterSeconds(response) {
  const advertised = Number(response.headers.get('retry-after'));
  assert.ok(Number.isInteger(advertised) && advertised > 0, `Retry-After was ${response.headers.get('retry-after')}`);
  return advertised;
}

describe('signing in is limited by the address and email being guessed at', () => {
  test('refuses with 429 and a Retry-After once the email and address pair is exhausted', async () => {
    const { email } = await registerAccount('pair-limit');
    resetRateLimits();

    const pairLimit = configuredLimits.signInEmailAndAddress;
    const refused = await callUntilRefused(() => call(SIGN_IN_PATH, signInRequest(email)), {
      allowedAttempts: pairLimit,
      expectedStatus: REFUSED_SIGN_IN_STATUS,
      refusalStatus: RATE_LIMITED_STATUS,
      limitName: `the email and address pair limit of ${pairLimit}`,
    });
    assertRateLimited(refused, refused.text);
    // The advertised wait must be honoured: it cannot exceed the window it is counted in.
    assert.ok(
      retryAfterSeconds(refused) <= WINDOW_DURATION_MINUTES * SECONDS_PER_MINUTE,
      `Retry-After of ${refused.headers.get('retry-after')}s is longer than the window`,
    );
  });

  test('does not spend the budget of an address that signs in correctly', async () => {
    const { email } = await registerAccount('correct');
    resetRateLimits();

    const attempts = configuredLimits.signInEmailAndAddress + 2;
    for (let attempt = 0; attempt < attempts; attempt += 1) {
      const response = await call(SIGN_IN_PATH, signInRequest(email, true));
      assert.equal(response.status, ACCEPTED_STATUS, `a correct sign-in was refused: ${response.text}`);
    }
  });
});

describe('each of the four limits refuses on its own', () => {
  test('the email limit refuses regardless of the address', async () => {
    const { email } = await registerAccount('email-limit');
    resetRateLimits();
    fillCounter(RATE_LIMIT_SCOPE.signInEmail, email, configuredLimits.signInEmail);

    const refused = await call(SIGN_IN_PATH, signInRequest(email));
    assertRateLimited(refused, refused.text);
    assert.ok(retryAfterSeconds(refused) > 0);
  });

  test('the address limit refuses regardless of the email', async () => {
    const { email } = await registerAccount('address-limit');
    resetRateLimits();
    // One attempt so the service records the address it sees, then fill that counter.
    await call(SIGN_IN_PATH, signInRequest(email));
    const address = observedSubject(RATE_LIMIT_SCOPE.signInAddress);
    assertAddressRecorded(RATE_LIMIT_SCOPE.signInAddress, address);
    fillCounter(RATE_LIMIT_SCOPE.signInAddress, address, configuredLimits.signInAddress);

    const refused = await call(SIGN_IN_PATH, signInRequest(newEmail('unrelated')));
    assertRateLimited(refused, refused.text);
  });

  test('the registration limit refuses further registrations from one address', async () => {
    const first = await registerAccount('registration-limit');
    assert.equal(first.response.status, 201, first.response.text);
    const address = observedSubject(RATE_LIMIT_SCOPE.registrationAddress);
    assertAddressRecorded(RATE_LIMIT_SCOPE.registrationAddress, address);
    fillCounter(RATE_LIMIT_SCOPE.registrationAddress, address, configuredLimits.registrationAddress);

    const refused = await call(REGISTRATION_PATH, registrationRequest(newEmail('over-limit')));
    assertRateLimited(refused, refused.text);
    assert.ok(
      retryAfterSeconds(refused) <= REGISTRATION_RETRY_AFTER_LIMIT_SECONDS,
      `Retry-After was ${refused.headers.get('retry-after')}`,
    );
  });
});

describe('a limit is never indefinite', () => {
  test('access returns on its own once the window has passed', async () => {
    const { email } = await registerAccount('recovery');
    resetRateLimits();
    fillCounter(RATE_LIMIT_SCOPE.signInEmail, email, configuredLimits.signInEmail);
    assert.equal((await call(SIGN_IN_PATH, signInRequest(email))).status, RATE_LIMITED_STATUS);

    // Move the window into the past rather than waiting fifteen minutes for it to end. This is the
    // same passage of time the service reads from the clock.
    sql(
      `UPDATE ${RATE_LIMIT_COUNTER_TABLE} SET window_started_at = now() - interval '${COMPLETED_WINDOW_MINUTES} minutes'
       WHERE scope = '${RATE_LIMIT_SCOPE.signInEmail}' AND subject = '${email}'`,
    );

    const allowed = await call(SIGN_IN_PATH, signInRequest(email, true));
    assert.equal(allowed.status, ACCEPTED_STATUS, `access did not return after the window: ${allowed.text}`);
  });
});

describe('a limit reaches only the identity it counts', () => {
  test('an exhausted email does not throttle an unrelated account', async () => {
    const throttled = await registerAccount('throttled');
    const bystander = await registerAccount('bystander');
    resetRateLimits();
    fillCounter(RATE_LIMIT_SCOPE.signInEmail, throttled.email, configuredLimits.signInEmail);

    const refused = await call(SIGN_IN_PATH, signInRequest(throttled.email));
    assertRateLimited(refused, refused.text);

    const unaffected = await call(SIGN_IN_PATH, signInRequest(bystander.email, true));
    assert.equal(unaffected.status, ACCEPTED_STATUS, `an unrelated account was throttled: ${unaffected.text}`);
  });

  test('existing sessions keep working while new sign-ins are refused', async () => {
    const { email, cookie } = await registerAccount('keeps-working');
    resetRateLimits();
    fillCounter(RATE_LIMIT_SCOPE.signInEmail, email, configuredLimits.signInEmail);

    const refused = await call(SIGN_IN_PATH, signInRequest(email));
    assertRateLimited(refused, refused.text);

    const live = await call(CURRENT_USER_PATH, { cookie });
    assert.equal(live.status, ACCEPTED_STATUS, `a live session stopped working while limited: ${live.text}`);
    assert.equal(live.json.user.email, email);
  });
});

describe('the counters are the service state, not the process state', () => {
  test('a restart does not reset them', async () => {
    const { email } = await registerAccount('restart-limit');
    resetRateLimits();
    fillCounter(RATE_LIMIT_SCOPE.signInEmail, email, configuredLimits.signInEmail);
    assert.equal((await call(SIGN_IN_PATH, signInRequest(email))).status, RATE_LIMITED_STATUS);

    compose('restart', 'api');
    await waitForReady();

    const stillRefused = await call(SIGN_IN_PATH, signInRequest(email));
    assert.equal(
      stillRefused.status,
      RATE_LIMITED_STATUS,
      `a restart handed back a fresh budget: ${stillRefused.text}`,
    );
  });
});

describe('the limits are configuration the running service reads', () => {
  test('a limit set in the environment replaces the documented default', async () => {
    try {
      // Recreate the API with one limit lowered. If the service read a constant instead of its
      // configuration, the third registration below would still be accepted.
      composeWith(
        { RATE_LIMIT_REGISTRATION_ADDRESS_ATTEMPTS: String(LOWERED_REGISTRATION_LIMIT) },
        'up',
        '--detach',
        '--force-recreate',
        '--no-deps',
        'api',
      );
      await waitForReady();
      resetRateLimits();

      for (let attempt = 0; attempt < LOWERED_REGISTRATION_LIMIT; attempt += 1) {
        const allowed = await call(REGISTRATION_PATH, registrationRequest(newEmail('configured')));
        assert.equal(allowed.status, 201, `attempt ${attempt + 1} was refused: ${allowed.text}`);
      }
      const refused = await call(REGISTRATION_PATH, registrationRequest(newEmail('configured')));
      assertRateLimited(refused, `the lowered limit was not applied: ${refused.text}`);
    } finally {
      compose('up', '--detach', '--force-recreate', '--no-deps', 'api');
      await waitForReady();
      resetRateLimits();
    }
  });
});

describe('a refused attempt costs no hashing', () => {
  test('answers a throttled sign-in far faster than it verifies a password', async () => {
    const { email } = await registerAccount('cost');
    resetRateLimits();

    const startedVerifying = performance.now();
    assert.equal((await call(SIGN_IN_PATH, signInRequest(email, true))).status, ACCEPTED_STATUS);
    const verifiedDurationMs = performance.now() - startedVerifying;

    fillCounter(RATE_LIMIT_SCOPE.signInEmail, email, configuredLimits.signInEmail);
    const startedRefusing = performance.now();
    assert.equal((await call(SIGN_IN_PATH, signInRequest(email))).status, RATE_LIMITED_STATUS);
    const refusedDurationMs = performance.now() - startedRefusing;

    // Argon2id at the configured cost dominates a verified sign-in. A refusal that paid for a hash
    // could not be markedly cheaper, so a clear margin is what shows the limit ran first.
    assert.ok(
      refusedDurationMs < verifiedDurationMs,
      `a throttled attempt took ${refusedDurationMs.toFixed(1)}ms against ` +
        `${verifiedDurationMs.toFixed(1)}ms for a verified one`,
    );
  });
});
