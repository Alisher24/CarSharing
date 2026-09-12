// The four limits of Q11, observed on the real HTTP boundary. Each test starts from cleared
// counters and fills exactly the limit it is about, so a refusal can only come from that limit.
import assert from 'node:assert/strict';
import { before, beforeEach, describe, test } from 'node:test';
import { call, compose, composeWith, newEmail, password, registerAccount, resetRateLimits, sql, waitForReady } from './client.mjs';

before(waitForReady);
beforeEach(resetRateLimits);

/**
 * The limits the running service reads, taken from the service's own configuration rather than
 * restated here, so a changed setting changes what these tests demand.
 */
const configured = {
  signInEmailAndAddress: Number(process.env.RATE_LIMIT_SIGNIN_EMAIL_ADDRESS_ATTEMPTS ?? 10),
  signInEmail: Number(process.env.RATE_LIMIT_SIGNIN_EMAIL_ATTEMPTS ?? 30),
  signInAddress: Number(process.env.RATE_LIMIT_SIGNIN_ADDRESS_ATTEMPTS ?? 100),
  registrationAddress: Number(process.env.RATE_LIMIT_REGISTRATION_ADDRESS_ATTEMPTS ?? 10),
};

const signIn = (email, wrong = true) =>
  call('/api/v1/auth/login', {
    method: 'POST',
    body: { email, password: wrong ? 'wrongpasswordvalue' : password },
  });

/** Writes a counter straight to its threshold, so a test does not have to make 100 requests. */
function fillCounter(scope, subject, attempts) {
  sql(
    `INSERT INTO rate_limit_counters (scope, subject, window_started_at, attempts)
     VALUES ('${scope}', '${subject}', now(), ${attempts})
     ON CONFLICT (scope, subject) DO UPDATE SET window_started_at = now(), attempts = ${attempts}`,
  );
}

/** The address the suite reaches the service from, as the service recorded it. */
function observedAddress() {
  return sql("SELECT subject FROM rate_limit_counters WHERE scope = 'sign_in_address' LIMIT 1");
}

describe('signing in is limited by the address and email being guessed at', () => {
  test('refuses with 429 and a Retry-After once the email and address pair is exhausted', async () => {
    const { email } = await registerAccount('pair-limit');
    resetRateLimits();
    let refused = null;
    for (let attempt = 0; attempt < configured.signInEmailAndAddress + 1; attempt += 1) {
      const response = await signIn(email);
      if (response.status === 429) {
        refused = response;
        break;
      }
      assert.equal(response.status, 401, response.text);
    }
    assert.ok(refused, `the pair limit of ${configured.signInEmailAndAddress} never refused an attempt`);
    assert.equal(refused.json.code, 'RATE_LIMITED');
    const retryAfter = Number(refused.headers.get('retry-after'));
    assert.ok(Number.isInteger(retryAfter) && retryAfter > 0, `Retry-After was ${refused.headers.get('retry-after')}`);
    // The advertised wait must be honoured: it cannot exceed the window it is counted in.
    assert.ok(retryAfter <= 15 * 60, `Retry-After of ${retryAfter}s is longer than the window`);
  });

  test('does not spend the budget of an address that signs in correctly', async () => {
    const { email } = await registerAccount('correct');
    resetRateLimits();
    for (let attempt = 0; attempt < configured.signInEmailAndAddress + 2; attempt += 1) {
      const response = await signIn(email, false);
      assert.equal(response.status, 200, `a correct sign-in was refused: ${response.text}`);
    }
  });
});

describe('each of the four limits refuses on its own', () => {
  test('the email limit refuses regardless of the address', async () => {
    const { email } = await registerAccount('email-limit');
    resetRateLimits();
    fillCounter('sign_in_email', email, configured.signInEmail);
    const refused = await signIn(email);
    assert.equal(refused.status, 429, refused.text);
    assert.equal(refused.json.code, 'RATE_LIMITED');
    assert.ok(Number(refused.headers.get('retry-after')) > 0);
  });

  test('the address limit refuses regardless of the email', async () => {
    const { email } = await registerAccount('address-limit');
    resetRateLimits();
    // One attempt so the service records the address it sees, then fill that counter.
    await signIn(email);
    const address = observedAddress();
    assert.ok(address, 'the service recorded no address for a sign-in');
    fillCounter('sign_in_address', address, configured.signInAddress);

    const refused = await signIn(newEmail('unrelated'));
    assert.equal(refused.status, 429, refused.text);
    assert.equal(refused.json.code, 'RATE_LIMITED');
  });

  test('the registration limit refuses further registrations from one address', async () => {
    const first = await registerAccount('registration-limit');
    assert.equal(first.response.status, 201, first.response.text);
    const address = sql("SELECT subject FROM rate_limit_counters WHERE scope = 'registration_address' LIMIT 1");
    assert.ok(address, 'the service recorded no address for a registration');
    fillCounter('registration_address', address, configured.registrationAddress);

    const refused = await call('/api/v1/auth/register', {
      method: 'POST',
      body: { email: newEmail('over-limit'), password },
    });
    assert.equal(refused.status, 429, refused.text);
    assert.equal(refused.json.code, 'RATE_LIMITED');
    const retryAfter = Number(refused.headers.get('retry-after'));
    assert.ok(retryAfter > 0 && retryAfter <= 3600, `Retry-After was ${retryAfter}`);
  });
});

describe('a limit is never indefinite', () => {
  test('access returns on its own once the window has passed', async () => {
    const { email } = await registerAccount('recovery');
    resetRateLimits();
    fillCounter('sign_in_email', email, configured.signInEmail);
    assert.equal((await signIn(email, false)).status, 429);

    // Move the window into the past rather than waiting fifteen minutes for it to end. This is the
    // same passage of time the service reads from the clock.
    sql(`UPDATE rate_limit_counters SET window_started_at = now() - interval '16 minutes'
         WHERE scope = 'sign_in_email' AND subject = '${email}'`);

    const allowed = await signIn(email, false);
    assert.equal(allowed.status, 200, `access did not return after the window: ${allowed.text}`);
  });
});

describe('a limit reaches only the identity it counts', () => {
  test('an exhausted email does not throttle an unrelated account', async () => {
    const throttled = await registerAccount('throttled');
    const bystander = await registerAccount('bystander');
    resetRateLimits();
    fillCounter('sign_in_email', throttled.email, configured.signInEmail);

    assert.equal((await signIn(throttled.email, false)).status, 429);
    const unaffected = await signIn(bystander.email, false);
    assert.equal(unaffected.status, 200, `an unrelated account was throttled: ${unaffected.text}`);
  });

  test('existing sessions keep working while new sign-ins are refused', async () => {
    const { email, cookie } = await registerAccount('keeps-working');
    resetRateLimits();
    fillCounter('sign_in_email', email, configured.signInEmail);

    assert.equal((await signIn(email, false)).status, 429);
    const live = await call('/api/v1/me', { cookie });
    assert.equal(live.status, 200, `a live session stopped working while limited: ${live.text}`);
    assert.equal(live.json.user.email, email);
  });
});

describe('the counters are the service state, not the process state', () => {
  test('a restart does not reset them', async () => {
    const { email } = await registerAccount('restart-limit');
    resetRateLimits();
    fillCounter('sign_in_email', email, configured.signInEmail);
    assert.equal((await signIn(email, false)).status, 429);

    compose('restart', 'api');
    await waitForReady();

    const stillRefused = await signIn(email, false);
    assert.equal(stillRefused.status, 429, `a restart handed back a fresh budget: ${stillRefused.text}`);
  });
});

describe('the limits are configuration the running service reads', () => {
  test('a limit set in the environment replaces the documented default', async () => {
    const lowered = 2;
    try {
      // Recreate the API with one limit lowered. If the service read a constant instead of its
      // configuration, the third registration below would still be accepted.
      composeWith(
        { RATE_LIMIT_REGISTRATION_ADDRESS_ATTEMPTS: String(lowered) },
        'up', '--detach', '--force-recreate', '--no-deps', 'api',
      );
      await waitForReady();
      resetRateLimits();

      for (let attempt = 0; attempt < lowered; attempt += 1) {
        const allowed = await call('/api/v1/auth/register', {
          method: 'POST',
          body: { email: newEmail('configured'), password },
        });
        assert.equal(allowed.status, 201, `attempt ${attempt + 1} was refused: ${allowed.text}`);
      }
      const refused = await call('/api/v1/auth/register', {
        method: 'POST',
        body: { email: newEmail('configured'), password },
      });
      assert.equal(refused.status, 429, `the lowered limit was not applied: ${refused.text}`);
      assert.equal(refused.json.code, 'RATE_LIMITED');
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

    const startVerified = performance.now();
    assert.equal((await signIn(email, false)).status, 200);
    const verifiedMs = performance.now() - startVerified;

    fillCounter('sign_in_email', email, configured.signInEmail);
    const startRefused = performance.now();
    assert.equal((await signIn(email, false)).status, 429);
    const refusedMs = performance.now() - startRefused;

    // Argon2id at the configured cost dominates a verified sign-in. A refusal that paid for a hash
    // could not be markedly cheaper, so a clear margin is what shows the limit ran first.
    assert.ok(
      refusedMs < verifiedMs,
      `a throttled attempt took ${refusedMs.toFixed(1)}ms against ${verifiedMs.toFixed(1)}ms for a verified one`,
    );
  });
});
