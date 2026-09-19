// What a burst of simultaneous sign-ins does to the sign-in budget, observed on the real HTTP
// boundary. The budget is spent by the same database statement that decides whether an attempt fits,
// so the number of passwords a burst can have verified is the budget itself and not the budget plus
// the number of requests that were in flight together.
import assert from 'node:assert/strict';
import { after, before, describe, test } from 'node:test';
import {
  call,
  newEmail,
  registerAccount,
  resetRateLimits,
  SIGN_IN_PATH,
  signInRequest,
  sql,
  waitForReady,
  settleAfterBurst,
} from './client.mjs';

/** The limits the running service reads, taken from its own configuration rather than restated. */
const pairLimit = Number(process.env.RATE_LIMIT_SIGNIN_EMAIL_ADDRESS_ATTEMPTS ?? 10);
const addressLimit = Number(process.env.RATE_LIMIT_SIGNIN_ADDRESS_ATTEMPTS ?? 100);

/**
 * How many attempts one burst sends. It is several times the pair budget, so a service that decided
 * each attempt before counting any of them would verify that many passwords.
 */
const BURST_SIZE = 40;

const RATE_LIMITED_STATUS = 429;
const REFUSED_CREDENTIALS_STATUS = 401;
const ACCEPTED_STATUS = 200;
const UNAVAILABLE_STATUS = 503;
const RATE_LIMITED_CODE = 'RATE_LIMITED';
const SERVICE_UNAVAILABLE_CODE = 'SERVICE_UNAVAILABLE';
const SIGN_IN_PAIR_SCOPE = 'sign_in_email_address';
const SIGN_IN_EMAIL_SCOPE = 'sign_in_email';
const SIGN_IN_ADDRESS_SCOPE = 'sign_in_address';
const SESSION_TABLE = 'sessions';
const SLOWEST_ACCEPTABLE_REFUSAL_MS = 10_000;

/** The attempts one counter holds, or zero when no attempt has reached it. */
function counterAttempts(scope, subject) {
  const attempts = sql(`SELECT attempts FROM rate_limit_counters WHERE scope = '${scope}' AND subject = '${subject}'`);
  return Number(attempts || 0);
}

/** The one subject the service recorded for a scope, which is the client address it observed. */
function observedSubject(scope) {
  const subject = sql(`SELECT subject FROM rate_limit_counters WHERE scope = '${scope}' LIMIT 1`);
  assert.ok(subject, `the service recorded no subject for scope ${scope}`);
  return subject;
}

function countRows(query) {
  const counted = Number(sql(query));
  assert.ok(Number.isInteger(counted), `the count of ${query} was not a number: ${counted}`);
  return counted;
}

/**
 * The slow answers of a burst are the ones that reached a password check, so the percentiles of the
 * burst say how much of that work it released. A refusal is answered without one.
 */
function percentile(durations, fraction) {
  const ordered = [...durations].sort((left, right) => left - right);
  const position = Math.min(ordered.length - 1, Math.ceil(fraction * ordered.length) - 1);
  return ordered[position];
}

/** Sends one burst and reports every answer with the time it took the service to give it. */
async function sendBurst(email, size = BURST_SIZE, passwordIsCorrect = false) {
  const startedAt = performance.now();
  const answers = await Promise.all(
    Array.from({ length: size }, async () => {
      const attemptStartedAt = performance.now();
      const response = await call(SIGN_IN_PATH, signInRequest(email, passwordIsCorrect));
      return { response, duration: performance.now() - attemptStartedAt };
    }),
  );
  return { answers, totalDuration: performance.now() - startedAt };
}

/**
 * Counts one burst by the answers it received. A wrong password that was verified answers 401 and an
 * attempt that reached a saturated hasher answers 503; both spent the budget of the email and address
 * pair, so both are counts of the password checks the burst released.
 */
function summarise(answers) {
  const statuses = answers.map(({ response }) => response.status);
  const verified = statuses.filter((status) => status === REFUSED_CREDENTIALS_STATUS).length;
  const hasherRefusals = statuses.filter((status) => status === UNAVAILABLE_STATUS).length;
  return {
    verified,
    hasherRefusals,
    claimed: verified + hasherRefusals,
    accepted: statuses.filter((status) => status === ACCEPTED_STATUS).length,
    refusedByLimit: statuses.filter((status) => status === RATE_LIMITED_STATUS).length,
    answered: statuses.length,
  };
}

/** Fails with the measurement when a refusal is not the documented refusal it must be. */
function assertRefusalsAreDocumented(answers, measured) {
  for (const { response } of answers.filter(({ response }) => response.status === RATE_LIMITED_STATUS)) {
    assert.equal(response.json.code, RATE_LIMITED_CODE, `${measured}: ${response.text}`);
    const advertised = Number(response.headers.get('retry-after'));
    assert.ok(
      Number.isInteger(advertised) && advertised > 0,
      `${measured}: Retry-After was ${response.headers.get('retry-after')}`,
    );
  }
  for (const { response } of answers.filter(({ response }) => response.status === UNAVAILABLE_STATUS)) {
    assert.equal(response.json.code, SERVICE_UNAVAILABLE_CODE, `${measured}: ${response.text}`);
  }
}

// A burst is this suite's subject, and the suites after it inherit what it left: the wait is the
// harness standing between the burst and the next reader of the same instance.
after(settleAfterBurst);

before(waitForReady);

describe('a burst of simultaneous sign-ins does not release more password checks than the budget', () => {
  test('forty at once against one email and address reach the password no more often than the budget', async () => {
    resetRateLimits();
    const { email } = await registerAccount('burst');
    resetRateLimits();
    // One attempt first, so the address the service sees is recorded and the pair budget stands one
    // short of its limit: a burst that fits it must be refused rather than merely finish it.
    assert.equal((await call(SIGN_IN_PATH, signInRequest(email))).status, REFUSED_CREDENTIALS_STATUS);
    const address = observedSubject(SIGN_IN_ADDRESS_SCOPE);
    const sessionsBefore = countRows(`SELECT count(*) FROM ${SESSION_TABLE}`);

    const { answers, totalDuration } = await sendBurst(email);
    const counted = summarise(answers);
    const pairCounter = counterAttempts(SIGN_IN_PAIR_SCOPE, `${email} ${address}`);
    const emailCounter = counterAttempts(SIGN_IN_EMAIL_SCOPE, email);
    const addressCounter = counterAttempts(SIGN_IN_ADDRESS_SCOPE, address);
    const sessionsAfter = countRows(`SELECT count(*) FROM ${SESSION_TABLE}`);
    const durations = answers.map(({ duration }) => duration);
    const measured = [
      `burst=${BURST_SIZE}`,
      `budget=${pairLimit}`,
      `verified=${counted.verified}`,
      `hasher_refusals=${counted.hasherRefusals}`,
      `claimed=${counted.claimed}`,
      `refused=${counted.refusedByLimit}`,
      `pair_counter=${pairCounter}`,
      `address_counter=${addressCounter}`,
      `sessions_added=${sessionsAfter - sessionsBefore}`,
      `p50=${percentile(durations, 0.5).toFixed(1)}ms`,
      `p95=${percentile(durations, 0.95).toFixed(1)}ms`,
      `burst_total=${totalDuration.toFixed(1)}ms`,
    ].join(' ');
    process.stdout.write(`sign-in burst: ${measured}\n`);

    assertRefusalsAreDocumented(answers, measured);
    assert.equal(counted.answered, BURST_SIZE, `a burst attempt had no answer: ${measured}`);
    assert.equal(counted.accepted, 0, `a wrong password was accepted: ${measured}`);
    // The one attempt before the burst spent one of the budget, so exactly what is left of it may
    // reach a password — however many requests arrive together. This is the assertion the suite exists
    // for: before the claim was one statement, every one of the forty attempts was told the budget had
    // room and forty passwords were verified.
    assert.equal(
      counted.claimed,
      pairLimit - 1,
      `the burst claimed ${counted.claimed} attempts against a budget of ${pairLimit}: ${measured}`,
    );
    // The pair's counter is the same claim recorded in the database: it states the attempts its window
    // had decided, so it stops at the budget — a refused attempt leaves it where the budget is rather
    // than raising it past.
    assert.ok(pairCounter <= pairLimit, `the pair counter passed the budget: ${measured}`);
    // A session belongs to a sign-in that proved a password, and no attempt of this burst did, so the
    // table is exactly as long as it was before the burst.
    assert.equal(
      sessionsAfter,
      sessionsBefore,
      `the burst added ${sessionsAfter - sessionsBefore} sessions: ${measured}`,
    );
    // An attempt the pair refused never reached the email's budget or the address's, so each of those
    // counters stands exactly where the attempts that got through left it — the other half of what the
    // pair's own counter states.
    assert.equal(
      emailCounter,
      counted.claimed,
      `the attempts that reached a password were not counted against the email: ${measured}`,
    );
    assert.equal(
      addressCounter,
      counted.claimed,
      `the attempts that reached a password were not counted against the address: ${measured}`,
    );
    assert.ok(
      percentile(durations, 0.95) < SLOWEST_ACCEPTABLE_REFUSAL_MS,
      `the burst queued instead of being refused: ${measured}`,
    );
  });

  test('a proven password inside a burst does not hand the budget to guessing', async () => {
    resetRateLimits();
    const { email } = await registerAccount('burst-correct');
    resetRateLimits();

    const { answers } = await sendBurst(email, BURST_SIZE + 1, true);
    const counted = summarise(answers);
    const address = observedSubject(SIGN_IN_ADDRESS_SCOPE);
    const pairCounter = counterAttempts(SIGN_IN_PAIR_SCOPE, `${email} ${address}`);
    const measured =
      `accepted=${counted.accepted} verified=${counted.verified} hasher_refusals=${counted.hasherRefusals} ` +
      `refused=${counted.refusedByLimit} pair_counter=${pairCounter} budget=${pairLimit}`;
    process.stdout.write(`sign-in burst with a proven password: ${measured}\n`);

    assert.equal(counted.answered, BURST_SIZE + 1, `a burst attempt had no answer: ${measured}`);
    assert.equal(counted.claimed + counted.refusedByLimit + counted.accepted, BURST_SIZE + 1, measured);
    assert.ok(pairCounter <= pairLimit, `the burst spent more than the budget: ${measured}`);

    // The same account once more, guessing, from an address whose own budget is untouched. A correct
    // sign-in that gave its attempt back must not have made room for a second budget of guesses: what
    // may reach a password is one budget, whatever was proved before it.
    const { answers: guesses } = await sendBurst(email, BURST_SIZE);
    const guessed = summarise(guesses);
    const measuredGuesses =
      `verified=${guessed.verified} hasher_refusals=${guessed.hasherRefusals} claimed=${guessed.claimed} ` +
      `refused=${guessed.refusedByLimit} pair_counter=${counterAttempts(SIGN_IN_PAIR_SCOPE, `${email} ${address}`)}`;
    process.stdout.write(`sign-in guesses after a proven password: ${measuredGuesses}\n`);

    assert.equal(guessed.answered, BURST_SIZE, `a guess had no answer: ${measuredGuesses}`);
    // A claim is spent until the credentials are proved, and a refused claim spends nothing, so the
    // claims of both bursts together are what the budget of this pair paid for — whatever mix of
    // guesses and proven passwords made them.
    assert.ok(
      counted.claimed + guessed.claimed <= pairLimit,
      `the pair spent ${counted.claimed + guessed.claimed} claims against a budget of ${pairLimit}: ` +
        `${measured} ${measuredGuesses}`,
    );
    assert.ok(
      counterAttempts(SIGN_IN_PAIR_SCOPE, `${email} ${address}`) <= pairLimit,
      `the guesses passed the budget: ${measuredGuesses}`,
    );
  });

  test('an exhausted address is refused without spending the pair budget', async () => {
    resetRateLimits();
    const { email } = await registerAccount('burst-address');
    resetRateLimits();
    assert.equal((await call(SIGN_IN_PATH, signInRequest(email))).status, REFUSED_CREDENTIALS_STATUS);
    const address = observedSubject(SIGN_IN_ADDRESS_SCOPE);
    sql(
      `INSERT INTO rate_limit_counters (scope, subject, window_started_at, attempts)
       VALUES ('${SIGN_IN_ADDRESS_SCOPE}', '${address}', now(), ${addressLimit})
       ON CONFLICT (scope, subject) DO UPDATE SET attempts = ${addressLimit}, window_started_at = now()`,
    );

    const { answers, totalDuration } = await sendBurst(newEmail('burst-other'), 5);
    const counted = summarise(answers);
    const measured =
      `refused=${counted.refusedByLimit} claimed=${counted.claimed} ` + `burst_total=${totalDuration.toFixed(1)}ms`;
    process.stdout.write(`sign-in burst against an exhausted address: ${measured}\n`);

    assert.equal(counted.refusedByLimit, 5, `an exhausted address let attempts through: ${measured}`);
    assert.equal(counted.claimed, 0, `an exhausted address paid for a password check: ${measured}`);
    assert.ok(
      totalDuration < SLOWEST_ACCEPTABLE_REFUSAL_MS,
      `five refusals took longer than a password check: ${measured}`,
    );
  });
});
