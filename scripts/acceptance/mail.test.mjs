// Delivering the letter with an invoice on the assembled stack: one letter per delivery key, read
// through the mailbox a person uses, together with the internal listener and the demonstration
// control that arms the loss of an answer.
//
// Everything a check observes is a stored row, an HTTP answer or a row of the box rather than a line
// of a log: a stub that lost a letter quietly is indistinguishable from a working one, so the record
// is read from the mail table and from the mailbox both.
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { setTimeout as delay } from 'node:timers/promises';
import { after, before, beforeEach, describe, test } from 'node:test';
import { compose, sql } from '../service.mjs';
import { serviceOrigin, waitForReady } from './client.mjs';
import {
  APPLICATION_ROLE,
  DELIVERY_PATH,
  INBOX_PATH,
  MAIL_ACTION_PATH,
  MAIL_ROLE,
  MESSAGES_PATH,
  armLostAnswer,
  armedFaults,
  asApplication,
  asMailRole,
  call,
  deliveredLetter,
  deliveryKeyOf,
  forgetSuiteMail,
  inboxLetterPath,
  internalCall,
  letterTask,
  letters,
  lettersAbout,
  mailbox,
  messagePath,
  rememberedActions,
  startMailstub,
  startWorker,
  stopMailstub,
  storedLetter,
  until,
} from './mail.mjs';
import { demandOutcome, prepareFreeRide, restoreRentalRates, storedPayment } from './payments.mjs';
import {
  IDEMPOTENCY_HEADER,
  availableVehicle,
  endSuiteReservations,
  newAccount,
  newCommandKey,
  reserve,
  restoreScenario,
} from './reservations.mjs';
import { FIRST_VEHICLE, demoCommand, stopSimulator, storedEnding, until as untilSimulated } from './simulation.mjs';

/** The accounts this suite registers, which is also how it recognizes its own rows afterwards. */
const ACCOUNT_PREFIX = 'mail';

/** Where a ride is started and ended, and where the invoice of one is paid. */
const startPath = (rentalId) => `/api/v1/reservations/${rentalId}/start`;
const pausePath = (rentalId) => `/api/v1/rides/${rentalId}/pause`;
const finishPath = (rentalId) => `/api/v1/rides/${rentalId}/finish`;
const payPath = (invoiceId) => `/api/v1/me/invoices/${invoiceId}/pay`;

/** The moment the contract publishes, which every answered moment is compared against. */
const MOMENT = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/;

/** The source the demonstration's first group of vehicles carries, and what a drained one holds. */
const BATTERY = 'battery';
const DRAINED_TO = '20';

/** Some identifier of the shape a delivery key names, for the checks that never reach the box. */
const ANY_INVOICE = '01994342-6ba7-7000-8000-000000000001';

/** How many tyiyn make one som, which is how the checks render an amount the way the letter does. */
const TYIYN_IN_SOM = 100n;

before(async () => {
  await waitForReady();
});

// Every check ends its own rides, so what the previous one left is removed before the next starts: a
// letter about another check's invoice would make a count about one invoice meaningless. The worker
// is put back in case a check stopped it.
beforeEach(async () => {
  await endSuiteMail();
});

after(async () => {
  await endSuiteMail();
  restoreScenario();
});

describe('the letter of a finished ride', () => {
  test('is exactly one letter naming the ending, the total and both lines of the invoice', async () => {
    const { account, rentalId } = await riding('ordinary');
    const finished = await finishCommand(rentalId, account);
    assert.equal(finished.status, 200, finished.text);

    const invoice = finished.json.invoice.invoice;
    const letter = await deliveredLetter(invoice.id);

    // The letter is addressed to the owner of the invoice and says what happened.
    assert.equal(letter.to, account.email, 'the letter went to somebody other than the owner');
    assert.match(letter.subject, /завершен/i, `the subject names no ending: ${letter.subject}`);
    assert.match(letter.acceptedAt, MOMENT);
    assert.ok(!letter.text.includes('invoice:'), 'the letter publishes an internal identifier');
    assert.ok(letter.text.includes('поездку завершил пользователь'), `the letter names no reason:\n${letter.text}`);

    // What it states is the invoice the API published rather than a sum computed again.
    assert.ok(
      letter.text.includes(`Итог: ${somText(invoice.total_amount_tyiyn)}`),
      `the letter states another total:\n${letter.text}`,
    );
    for (const line of invoice.lines) {
      const mode = line.mode === 'driving' ? 'Движение' : 'Пауза';
      assert.ok(
        letter.text.includes(
          `${mode}: ${line.billed_started_minutes} мин по ` +
            `${somText(line.rate_tyiyn_per_started_minute)} за минуту = ${somText(line.amount_tyiyn)}`,
        ),
        `the letter does not state the ${line.mode} line of the invoice:\n${letter.text}`,
      );
    }
    assert.equal(lettersAbout(invoice.id), 1, 'the ending produced more than one letter');

    // The record is read through both operations of the box, which is what makes the letter evidence
    // rather than an assertion about the process that wrote it.
    const one = await mailbox(messagePath(letter.id));
    assert.equal(one.status, 200, one.text);
    assert.equal(one.json.text, letter.text, 'the box answers another text than the table holds');
    assert.equal(one.json.to, account.email);

    const page = await letters('?limit=100');
    const summary = page.items.find((item) => item.id === letter.id);
    assert.notEqual(summary, undefined, 'the collection does not publish the letter');
    assert.equal(summary.subject, letter.subject);
    assert.equal(summary.accepted_at, letter.acceptedAt);
    assert.equal(summary.text, undefined, 'the summary of a letter carries its whole text');
  });

  test('names the reason and the sources that ran out when the ride ended by itself', async () => {
    compose('--profile', 'demo', 'up', '--detach', 'simulator');
    try {
      const { account, vehicleId, rentalId } = await riding('depleted', await freeVehicle());
      await demoCommand('set-energy-remaining', '--vehicle', vehicleId, '--source', BATTERY, '--remaining', DRAINED_TO);
      const ended = await untilSimulated(
        () => storedEnding(rentalId),
        'the ride whose battery ran out was never ended',
      );
      assert.equal(ended.reason, 'energy_depleted');
      assert.deepEqual(ended.sources, [BATTERY]);

      const invoiceId = sql(`SELECT id FROM invoices WHERE rental_id = '${rentalId}'`);
      const published = await call(`/api/v1/me/invoices/${invoiceId}`, { cookie: account.cookie });
      assert.equal(published.status, 200, published.text);
      assert.equal(published.json.invoice.completion.reason, 'energy_depleted');

      const letter = await deliveredLetter(invoiceId);
      assert.ok(
        letter.text.includes('закончился запас энергии или топлива: батарея'),
        `the letter does not name the source that ran out:\n${letter.text}`,
      );
      assert.ok(
        letter.text.includes(`Итог: ${somText(published.json.invoice.total_amount_tyiyn)}`),
        `the letter states another total than the API publishes:\n${letter.text}`,
      );
      assert.equal(lettersAbout(invoiceId), 1);

      await demoCommand('mark-serviced', '--vehicle', vehicleId);
    } finally {
      stopSimulator();
    }
  });
});

describe('repeating a delivery', () => {
  test('answers the receipt of the first one and stores no second letter', async () => {
    const { account, rentalId } = await riding('repeat');
    const finished = await finishCommand(rentalId, account);
    const invoiceId = finished.json.invoice.invoice.id;
    const delivered = await deliveredLetter(invoiceId);
    const deliveredAttempts = letterTask(invoiceId).attempts;

    // The queue is asked for the same delivery again, which is what an expired lease or a restarted
    // worker produces: the stub answers the receipt of the first delivery rather than storing a
    // second letter, and the task is confirmed on that answer.
    redeliver(invoiceId);
    await until(
      () => (letterTask(invoiceId).attempts > deliveredAttempts ? letterTask(invoiceId) : undefined),
      'the repeated delivery was never attempted',
    );
    await until(() => letterTask(invoiceId).completedAt !== undefined, 'the repeated delivery was never confirmed');
    assert.equal(lettersAbout(invoiceId), 1, 'the repeat stored a second letter');
    assert.deepEqual(storedLetter(invoiceId), delivered, 'the repeated delivery changed the letter');
  });

  test('stores one letter when two workers deliver the same task at once', async () => {
    const { account, rentalId } = await riding('two-workers');
    const finished = await finishCommand(rentalId, account);
    const invoiceId = finished.json.invoice.invoice.id;
    const delivered = await deliveredLetter(invoiceId);

    try {
      compose('up', '--detach', '--scale', 'worker=2', 'worker');
      redeliver(invoiceId);
      await until(() => letterTask(invoiceId).completedAt !== undefined, 'the redelivered task was never confirmed');
      // Both workers may attempt it; the key of the delivery is what makes the second attempt a
      // repeat rather than a second letter.
      await delay(2_000);
      assert.equal(lettersAbout(invoiceId), 1, 'two workers stored two letters');
      assert.deepEqual(storedLetter(invoiceId), delivered);
    } finally {
      compose('up', '--detach', '--scale', 'worker=1', 'worker');
    }
  });
});

describe('the loss of an answer', () => {
  test('leaves one stored letter that the retry is answered with', async () => {
    const action = armLostAnswer();
    assert.match(action.action_id, /^[0-9a-f-]{36}$/);
    assert.match(action.server_time, MOMENT);
    assert.equal(armedFaults(), 1, 'the action armed no demand');

    const { account, rentalId } = await riding('lost-answer');
    const finished = await finishCommand(rentalId, account);
    const invoiceId = finished.json.invoice.invoice.id;

    // The stub stored the letter and lost the answer, so the attempt failed and the task is retried
    // until the stub answers the receipt of the delivery it already holds.
    const delivered = await deliveredLetter(invoiceId);
    const settled = await until(
      () => (letterTask(invoiceId).completedAt === undefined ? undefined : letterTask(invoiceId)),
      'the retried delivery was never confirmed',
    );
    assert.ok(settled.attempts >= 2, `the delivery was not retried: ${settled.attempts} attempt(s)`);
    assert.equal(lettersAbout(invoiceId), 1, 'the retry stored a second letter');
    assert.deepEqual(storedLetter(invoiceId), delivered, 'the retry changed the stored letter');
    assert.equal(armedFaults(), 0, 'the demand was not spent by the delivery it decided');
    assert.deepEqual(rememberedActions(), [action.action_id]);
  });

  test('is spent by the delivery that stored a letter and not by a repeat of it', async () => {
    const { account, rentalId } = await riding('repeat-of-lost');
    const finished = await finishCommand(rentalId, account);
    const invoiceId = finished.json.invoice.invoice.id;
    await deliveredLetter(invoiceId);
    const attempts = letterTask(invoiceId).attempts;

    // A demand armed after the letter was stored belongs to the next delivery that stores one: the
    // repeat answers the receipt of the delivery that really arrived, so it loses nothing and takes
    // no demand away from the delivery still to come.
    armLostAnswer();
    assert.equal(armedFaults(), 1, 'the action armed no demand');
    redeliver(invoiceId);
    const repeated = await until(
      () => (letterTask(invoiceId).attempts > attempts ? letterTask(invoiceId) : undefined),
      'the repeated delivery was never attempted',
    );
    await until(() => letterTask(invoiceId).completedAt !== undefined, 'the repeated delivery was never confirmed');
    assert.equal(letterTask(invoiceId).attempts, repeated.attempts, 'the repeat was attempted twice');
    assert.equal(armedFaults(), 1, 'the repeat spent the demand that belonged to the next delivery');
    assert.equal(lettersAbout(invoiceId), 1, 'the repeat stored a second letter');
  });

  test('decides one delivery and is not armed again by a repeat of the action', async () => {
    const actionId = randomUUID();
    const armed = armLostAnswer(actionId);
    assert.equal(armed.action_id, actionId);

    // A repeat of the identifier reproduces the answer rather than arming a second loss: the demand
    // the first action made decides the next delivery, and the repeat adds none.
    const repeated = armLostAnswer(actionId);
    assert.equal(repeated.server_time, armed.server_time, 'the repeat answered another moment');
    assert.equal(armedFaults(), 1, 'the repeat armed a second demand');
    assert.deepEqual(rememberedActions(), [actionId]);

    const { account, rentalId } = await riding('after-loss');
    const finished = await finishCommand(rentalId, account);
    const invoiceId = finished.json.invoice.invoice.id;
    const delivered = await deliveredLetter(invoiceId);
    const settled = await until(
      () => (letterTask(invoiceId).completedAt === undefined ? undefined : letterTask(invoiceId)),
      'the delivery decided by the demand was never confirmed',
    );
    assert.ok(settled.attempts >= 2, `the lost answer was not retried: ${settled.attempts} attempt(s)`);
    assert.equal(armedFaults(), 0, 'one action armed more than one loss');
    assert.equal(lettersAbout(invoiceId), 1, 'the demand decided a second delivery');
    assert.deepEqual(storedLetter(invoiceId), delivered);
  });
});

describe('the letter and the payment of its invoice', () => {
  test('writes no second letter when the payment is refused and then repeated', async () => {
    const { account, rentalId } = await riding('payment');
    // The outcome of the attempt the ending owes is asked for before the ending, as the demonstration
    // control does it: the attempt the worker makes meets that demand and is refused.
    demandOutcome(rentalId, 'failed');
    const finished = await finishCommand(rentalId, account);
    const invoiceId = finished.json.invoice.invoice.id;
    await deliveredLetter(invoiceId);
    const written = storedLetter(invoiceId);
    const version = sql(`SELECT version FROM invoices WHERE id = '${invoiceId}'`);
    await until(
      () => (storedPayment(invoiceId).status === 'failed' ? true : undefined),
      'the demanded refusal never reached the payment',
    );

    // An attempt that settles the payment leaves the letter as the ending wrote it: the letter does
    // not depend on the outcome of the payment, whichever it is.
    const settled = await pay(invoiceId, newCommandKey(), account);
    assert.equal(settled.status, 200, settled.text);
    assert.equal(settled.json.invoice.payment.status, 'paid', settled.text);

    await delay(2_000);
    assert.equal(lettersAbout(invoiceId), 1, 'a payment transition produced a second letter');
    assert.deepEqual(storedLetter(invoiceId), written, 'a payment transition changed the letter');
    assert.equal(
      sql(`SELECT version FROM invoices WHERE id = '${invoiceId}'`),
      version,
      'the invoice moved with the payment of it',
    );
  });

  test('writes one letter about a ride that cost nothing and owes no attempt', async () => {
    const { account, rentalId } = await riding('zero');
    const rates = prepareFreeRide(rentalId);
    try {
      const finished = await finishCommand(rentalId, account);
      assert.equal(finished.status, 200, finished.text);
      const invoice = finished.json.invoice.invoice;
      assert.equal(invoice.total_amount_tyiyn, '0');
      assert.equal(finished.json.invoice.payment.status, 'paid');

      const letter = await deliveredLetter(invoice.id);
      assert.ok(letter.text.includes('Итог: 0,00 сома'), `the letter does not state zero:\n${letter.text}`);
      assert.ok(
        !letter.text.includes('ожидает оплаты'),
        `a ride that cost nothing waits for a payment:\n${letter.text}`,
      );
      assert.equal(awaitingPaymentAttempt(invoice.id), 0, 'a ride that cost nothing owes an attempt');
      assert.equal(lettersAbout(invoice.id), 1);
    } finally {
      restoreRentalRates(rentalId, rates);
    }
  });
});

describe('a delivery that cannot be made', () => {
  test('keeps the task in the queue and delivers one letter when the stub returns', async () => {
    stopMailstub();
    const { account, rentalId } = await riding('unreachable');
    const finished = await finishCommand(rentalId, account);
    const invoiceId = finished.json.invoice.invoice.id;

    const failed = await until(() => {
      const task = letterTask(invoiceId);
      return task !== undefined && task.attempts >= 1 && task.lastError !== '' ? task : undefined;
    }, 'the worker never recorded a failed delivery');
    assert.equal(failed.completedAt, undefined, 'a delivery that never arrived was confirmed');
    assert.match(failed.nextAttemptAt, MOMENT);
    assert.equal(storedLetter(invoiceId), undefined, 'a letter was stored although the stub was down');

    await startMailstub();
    const letter = await deliveredLetter(invoiceId);
    assert.equal(lettersAbout(invoiceId), 1, 'the stub stored more than one letter after it returned');
    assert.deepEqual(storedLetter(invoiceId), letter);
    await until(
      () => letterTask(invoiceId).completedAt !== undefined,
      'the delivery was never confirmed after the stub returned',
    );
  });
});

describe('the surface of the mail stub', () => {
  test('refuses a delivery without a credential and with a wrong one alike', () => {
    const key = deliveryKeyOf(ANY_INVOICE);
    const letter = JSON.stringify({ to: 'rider@example.test', subject: 'Письмо', text: 'Текст' });
    for (const refused of [
      { name: 'no credential', path: DELIVERY_PATH, options: { body: letter, deliveryKey: key } },
      {
        name: 'a wrong credential',
        path: DELIVERY_PATH,
        options: { body: letter, token: 'not-the-delivery-token', deliveryKey: key },
      },
      // The credential is checked before the payload is read, so a broken body changes nothing.
      {
        name: 'a malformed payload with no credential',
        path: DELIVERY_PATH,
        options: { body: '{', deliveryKey: key },
      },
      { name: 'a wrong credential on the action', path: MAIL_ACTION_PATH, options: { body: '{}' } },
    ]) {
      const answer = internalCall('POST', refused.path, refused.options);
      assert.equal(answer.status, 401, `${refused.name}: ${answer.text}`);
      assert.equal(JSON.parse(answer.text).code, 'INTERNAL_AUTHENTICATION_REQUIRED', refused.name);
    }
  });

  test('publishes no internal path to the host and no mail path through the proxy', async () => {
    assert.equal((await mailbox(DELIVERY_PATH, { method: 'POST' })).status, 404);
    assert.equal((await mailbox(MAIL_ACTION_PATH, { method: 'POST' })).status, 404);
    const proxied = await fetch(`${serviceOrigin}${DELIVERY_PATH}`, { method: 'POST' });
    assert.equal(proxied.status, 404, 'the proxy published an internal path of the mail stub');
    const mailThroughProxy = await fetch(`${serviceOrigin}${MESSAGES_PATH}`);
    assert.equal(mailThroughProxy.status, 404, 'the proxy published the mailbox');
  });

  test('has no operation that changes the box', async () => {
    for (const method of ['POST', 'PUT', 'DELETE']) {
      const answer = await mailbox(MESSAGES_PATH, { method });
      assert.equal(answer.status, 405, `${method} answered ${answer.status}`);
      assert.equal(answer.headers.get('allow'), 'GET', `${method} advertises another method`);
      assert.equal(answer.json.code, 'METHOD_NOT_ALLOWED');
    }
    const forged = await mailbox(`${MESSAGES_PATH}?limit=1&cursor=not-a-signed-cursor`);
    assert.equal(forged.status, 400, forged.text);
    assert.equal(forged.json.code, 'INVALID_CURSOR');
  });

  test('pages forward under its own key and refuses a cursor of another limit', async () => {
    // Two letters of this suite's own are delivered first, so the walk below reads a box it made
    // rather than whatever another suite left in it.
    for (const name of ['paging-one', 'paging-two']) {
      const { account, rentalId } = await riding(name);
      const finished = await finishCommand(rentalId, account);
      await deliveredLetter(finished.json.invoice.invoice.id);
    }

    // The whole box is walked one letter at a time: every page must continue after the letter the
    // previous one ended with, and the last page must state no cursor at all.
    let cursor;
    let read = 0;
    let previous;
    for (;;) {
      const query = cursor === undefined ? '?limit=1' : `?limit=1&cursor=${encodeURIComponent(cursor)}`;
      const page = await letters(query);
      assert.ok(page.items.length <= 1, `a page of one carried ${page.items.length} letters`);
      for (const item of page.items) {
        if (previous !== undefined) {
          assert.ok(
            item.accepted_at < previous.accepted_at ||
              (item.accepted_at === previous.accepted_at && item.id < previous.id),
            'the pages are not ordered by the pair the collection declares',
          );
        }
        previous = item;
        read += 1;
      }
      if (page.next_cursor === null) break;
      cursor = page.next_cursor;
    }
    assert.ok(read >= 1, 'the box published no letter at all');

    const first = await letters('?limit=1');
    assert.notEqual(first.next_cursor, null, 'the box holds more than one letter, so a cursor is owed');
    const wrongLimit = await mailbox(`${MESSAGES_PATH}?limit=2&cursor=${encodeURIComponent(first.next_cursor)}`);
    assert.equal(wrongLimit.status, 400, wrongLimit.text);
    assert.equal(wrongLimit.json.code, 'INVALID_CURSOR', 'a cursor of another limit was accepted');

    // One letter is read whole, and one that is not there is indistinguishable from one nobody wrote.
    const one = await mailbox(messagePath(first.items[0].id));
    assert.equal(one.status, 200, one.text);
    assert.equal(one.json.subject, first.items[0].subject);
    assert.notEqual(one.json.text, undefined, 'the box answers no text for a letter it published');
    assert.equal((await mailbox(messagePath(ANY_INVOICE))).status, 404);
  });

  test('shows the same letter as a page a person opens, and refuses as a page as well', async () => {
    const { account, rentalId } = await riding('page');
    const finished = await finishCommand(rentalId, account);
    const letter = await deliveredLetter(finished.json.invoice.invoice.id);
    assert.ok(letter.text.includes(somText(finished.json.invoice.invoice.total_amount_tyiyn)));

    // The list names the letter the box holds, and the page of it states what the stub stored: the
    // recipient, the moment it was accepted at in the zone it is stored in, the key it was delivered
    // under and the text as it is. Every one of them is escaped, so the page a person reads is HTML.
    const list = await mailbox(INBOX_PATH);
    assert.equal(list.status, 200, list.text);
    assert.match(list.headers.get('content-type'), /^text\/html/);
    assert.equal(list.headers.get('cache-control'), 'no-store');
    assert.equal(list.headers.get('x-content-type-options'), 'nosniff');
    assert.match(list.headers.get('content-security-policy'), /default-src 'none'/);
    assert.ok(list.text.includes(`href="${inboxLetterPath(letter.id)}"`), 'the list links to no letter');
    assert.ok(list.text.includes(letter.subject), `the list names no subject:\n${list.text}`);

    const page = await mailbox(inboxLetterPath(letter.id));
    assert.equal(page.status, 200, page.text);
    for (const stated of [
      letter.subject,
      letter.to,
      letter.id,
      deliveryKeyOf(finished.json.invoice.invoice.id),
      letter.text,
    ]) {
      assert.ok(page.text.includes(escapeMarkup(stated)), `the page does not state ${stated}:\n${page.text}`);
    }
    assert.ok(page.text.includes(`${letter.acceptedAt} UTC`), `the page states no stored moment:\n${page.text}`);

    // What the page does not hold is refused the way the operation is, so the box tells a reader
    // nothing the JSON does not.
    const absent = await mailbox(inboxLetterPath(ANY_INVOICE));
    assert.equal(absent.status, 404);
    assert.match(absent.headers.get('content-type'), /^text\/html/);
    const nowhere = await mailbox('/nowhere');
    assert.equal(nowhere.status, 404);
    assert.match(nowhere.headers.get('content-type'), /^text\/html/);
  });
});

describe('the privileges of the two roles', () => {
  test('lets the mail role write its own schema and reach nothing else', () => {
    const key = deliveryKeyOf(ANY_INVOICE);
    const written = asMailRole(
      `INSERT INTO mailstub.messages (id, delivery_key, recipient, subject, body, accepted_at)
       VALUES (uuidv7(), '${key}', 'rider@example.test', 'Тема', 'Текст', now())`,
    );
    assert.equal(written.accepted, true, written.refusal);
    assert.equal(asMailRole(`SELECT count(*) FROM mailstub.messages WHERE delivery_key = '${key}'`).accepted, true);
    assert.equal(asMailRole('UPDATE mailstub.messages SET subject = subject').accepted, false);
    assert.equal(asMailRole('DELETE FROM mailstub.messages').accepted, false);
    assert.equal(asMailRole('SELECT count(*) FROM public.rentals').accepted, false);
    assert.equal(asMailRole('SELECT count(*) FROM public.users').accepted, false);
    assert.equal(
      asMailRole('INSERT INTO mailstub.demo_delivery_faults (armed_at) VALUES (now())').accepted,
      false,
      'the role that serves mail can make the demand it is supposed to spend',
    );
    assert.equal(asMailRole('SELECT count(*) FROM mailstub.demo_delivery_faults').accepted, true);
    assert.equal(asMailRole('DELETE FROM mailstub.demo_delivery_faults').accepted, true);
    sql(`DELETE FROM mailstub.messages WHERE delivery_key = '${key}'`);
  });

  test('keeps the application out of the mail schema', () => {
    for (const statement of [
      'SELECT count(*) FROM mailstub.messages',
      'SELECT count(*) FROM mailstub.demo_actions',
      'INSERT INTO mailstub.messages (id, delivery_key, recipient, subject, body, accepted_at) ' +
        "VALUES (uuidv7(), 'invoice:x:issued', 'a@example.test', 's', 't', now())",
    ]) {
      const refused = asApplication(statement);
      assert.equal(refused.accepted, false, `${statement} was accepted by ${APPLICATION_ROLE}`);
    }
    assert.notEqual(MAIL_ROLE, APPLICATION_ROLE);
  });
});

/**
 * Registers an account and takes one free vehicle into a paused ride, which is the state every ending
 * below starts from.
 */
async function riding(name, chosenVehicle) {
  const account = await newAccount(`${ACCOUNT_PREFIX}-${name}`);
  const vehicleId = chosenVehicle ?? (await availableVehicle());
  const reserved = await reserve(vehicleId, newCommandKey(), account);
  assert.equal(reserved.status, 201, reserved.text);
  const rentalId = reserved.json.rental.id;

  const started = await rideCommand(startPath(rentalId), account);
  assert.equal(started.status, 200, started.text);
  const paused = await rideCommand(pausePath(rentalId), account);
  assert.equal(paused.status, 200, paused.text);
  return { account, vehicleId, rentalId };
}

/** Sends one command of a ride and answers what the API replied. */
function rideCommand(path, account) {
  return call(path, {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { [IDEMPOTENCY_HEADER]: newCommandKey() },
  });
}

/** Ends one ride and answers what the API replied, with the invoice of it. */
function finishCommand(rentalId, account) {
  return rideCommand(finishPath(rentalId), account);
}

/** Pays one invoice of the account that holds it. */
function pay(invoiceId, key, account) {
  return call(payPath(invoiceId), {
    method: 'POST',
    cookie: account.cookie,
    csrfToken: account.csrfToken,
    headers: { [IDEMPOTENCY_HEADER]: key },
  });
}

/**
 * Asks the queue for a delivery it has already made, which is what an expired lease or a restarted
 * worker produces: the task stands in the queue as one nobody has delivered.
 */
function redeliver(invoiceId) {
  sql(
    `UPDATE outbox
     SET completed_at = NULL, lease_token = NULL, lease_expires_at = NULL, next_attempt_at = now()
     WHERE kind = 'invoice.issued' AND resource_id = '${invoiceId}'`,
  );
}

/** How many attempts at the payment of one invoice the queue still owes, which a free ride owes none of. */
function awaitingPaymentAttempt(invoiceId) {
  return Number(
    sql(
      `SELECT count(*) FROM outbox
       WHERE kind = 'payment.attempt' AND resource_id = '${invoiceId}' AND completed_at IS NULL`,
    ),
  );
}

/** An amount of whole tyiyn written the way the letter writes it. */
function somText(tyiyn) {
  const amount = BigInt(tyiyn);
  const whole = amount / TYIYN_IN_SOM;
  const minor = (amount % TYIYN_IN_SOM).toString().padStart(2, '0');
  return `${groupedDigits(whole.toString())},${minor} сома`;
}

/**
 * What one value looks like inside the markup of a page. The stub decides nothing about what a letter
 * says, so the page shows a subject or a text as the characters it is; a check that looks for the
 * stored value in the markup escapes it the way the page does.
 */
function escapeMarkup(value) {
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll("'", '&#39;');
}

/** A whole number of som with its digits grouped in threes, as the letter groups them. */
function groupedDigits(digits) {
  let grouped = '';
  for (const [position, digit] of [...digits].entries()) {
    if (position > 0 && (digits.length - position) % 3 === 0) grouped += ' ';
    grouped += digit;
  }
  return grouped;
}

/**
 * One vehicle free to take: the first group's vehicle while the demonstration has left it free, which
 * is the one whose single source the check below drains, and any free vehicle otherwise.
 */
async function freeVehicle() {
  const published = await call(`/api/v1/vehicles/${FIRST_VEHICLE}`).catch(() => undefined);
  if (published?.status === 200 && published.json.status === 'available') return FIRST_VEHICLE;
  return availableVehicle();
}

/** Ends everything this suite wrote, so the suites that follow read the demonstration. */
async function endSuiteMail() {
  startWorker();
  await endSuiteReservations();
  forgetSuiteMail(ACCOUNT_PREFIX);
}
