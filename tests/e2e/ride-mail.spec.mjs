// What a person reads in the local inbox after a ride is over: the one letter that states the invoice,
// and no second letter however often the delivery is attempted.
//
// The letter is the surface no HTTP check can stand in for. An HTTP suite proves the row the mail stub
// stored and the page the listener answers; what this checks is that a person arrives at it the way
// the README says to — the inbox address, the letter the page lists, and the amount the page states
// matching what the panel says the ride cost.
//
// The retry is armed before the ending, so the delivery that stores the letter is the one whose answer
// is lost: the worker repeats it and the stub answers the receipt of the delivery it already holds.
import assert from 'node:assert/strict';
import { expect, test } from '@playwright/test';
import {
  armedFaults,
  armLostAnswer,
  forgetSuiteMail,
  INBOX_PATH,
  inboxLetterPath,
  letterTask,
  lettersAbout,
  mailbox,
  messagePath,
  storedLetter,
} from '../../scripts/acceptance/mail.mjs';
import { somText } from '../../scripts/acceptance/money.mjs';
import { MAILBOX_ORIGIN, sql } from '../../scripts/service.mjs';
import { availableModel, book, email, endRidesOf, RECONCILIATION_PATIENCE_MS, signUp, until } from './person.mjs';
import { restoreScenario } from './scenario.mjs';

/** The prefix of every account these checks register, which is how their rows are found again. */
const ACCOUNT_PREFIX = 'mail';

/** The controls of a ride, in the words the interface fixes for them. */
const START_ACTION = 'Начать поездку';
const FINISH_ACTION = 'Завершить поездку';

/** What the page of the inbox states itself to be, which every page of it carries in its header. */
const INBOX_TITLE = 'Ящик почтовой заглушки';

/**
 * What the panel says once the service confirmed that the ride is over, and what it writes before the
 * total.
 */
const RIDE_FINISHED = 'Поездка завершена';
const INVOICE_TOTAL = 'Итог счёта';

/** How long a check waits for a letter the queue owes, which is several times one delivery attempt. */
const DELIVERY_PATIENCE_MS = 60_000;

// A check ends the ride it made: the demonstration refuses to be put back while a rental of a
// person's stands on one of its vehicles, and the checks after it start from the prepared scenario.
// The letters about those rides go before the rides themselves: the box is read by joining its keys to
// the invoices, so a row removed first takes its letter out of reach of the cleanup.
test.beforeEach(() => {
  forgetSuiteMail(ACCOUNT_PREFIX);
  endRidesOf(ACCOUNT_PREFIX);
  restoreScenario();
  // Every check registers its own account, and the checks share one address. Clearing the counters is
  // the harness standing in for the passage of time, which is also how access returns in production.
  sql('DELETE FROM rate_limit_counters');
});

test.afterEach(() => {
  forgetSuiteMail(ACCOUNT_PREFIX);
  endRidesOf(ACCOUNT_PREFIX);
  restoreScenario();
});

test('one ride leaves one letter in the inbox, however often the delivery is repeated', async ({ browser }) => {
  // The stub loses the answer of the next delivery that stores a letter, which is the retry a person
  // reads about: the letter is stored, the answer is lost, and the worker asks again.
  const armed = armLostAnswer();
  assert.equal(armedFaults(), 1, 'the demonstration action armed no fault');

  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    await page.goto('/');
    await signUp(page, email(ACCOUNT_PREFIX));
    await book(page, await availableModel());
    await page.getByRole('button', { name: START_ACTION }).click();
    await page.getByRole('button', { name: FINISH_ACTION }).first().click();
    await page.locator('.reservation-panel-confirm').getByRole('button', { name: FINISH_ACTION }).click();
    await expect(page.locator('.reservation-panel-time')).toHaveText(RIDE_FINISHED, {
      timeout: RECONCILIATION_PATIENCE_MS,
    });

    // The letter the ride is owed reaches the inbox and is stored once, under the key of the invoice
    // the ending issued. The retry the armed fault caused is what the attempts of the task state.
    const invoiceId = invoiceIdOfCurrentRide();
    const letter = await deliveredLetter(invoiceId);
    const settled = await until(
      () => (letterTask(invoiceId).completedAt === undefined ? undefined : letterTask(invoiceId)),
      'the retried delivery was never confirmed',
    );
    assert.ok(settled.attempts >= 2, `the lost answer was not retried: ${settled.attempts} attempt(s)`);
    assert.equal(lettersAbout(invoiceId), 1, 'the retry stored a second letter');
    assert.equal(armedFaults(), 0, 'the fault was not spent by the delivery it decided');

    // The letter is the one the person finds in the inbox the README names, and it states the amount
    // the panel states for the same ride rather than a total computed by this check.
    const delivered = await mailbox(messagePath(letter.id));
    assert.equal(delivered.status, 200, delivered.text);
    assert.equal(delivered.json.id, letter.id);
    assert.equal(delivered.json.to, letter.to);
    assert.ok(
      delivered.json.text.includes(somText(totalOf(invoiceId))),
      `the letter states another total than the invoice:\n${delivered.json.text}`,
    );
    await expect(page.locator('.ride-progress')).toContainText(INVOICE_TOTAL);
    await expect(page.locator('.ride-progress')).toContainText(somText(totalOf(invoiceId)));

    // The same letter is read as a page in the browser a person opens: the list the inbox address
    // answers carries it, and the page of the letter states the total the panel states — read from
    // the page rather than from the JSON of the same letter.
    await page.goto(MAILBOX_ORIGIN + INBOX_PATH);
    await expect(page.locator('.inbox-letters')).toContainText(letter.subject);
    await page.getByRole('link', { name: letter.subject }).click();
    await expect(page).toHaveURL(MAILBOX_ORIGIN + inboxLetterPath(letter.id));
    await expect(page.locator('.inbox-text')).toContainText(somText(totalOf(invoiceId)));

    // A page reaches nothing outside itself, which is what its policy states and what a person's
    // browser really asks for: a context of its own records every request the page makes, and the
    // page it opens is the only one.
    const alone = await browser.newContext();
    try {
      const inbox = await alone.newPage();
      const asked = [];
      inbox.on('request', (request) => asked.push(request.url()));
      await inbox.goto(MAILBOX_ORIGIN + inboxLetterPath(letter.id));
      await expect(inbox.locator('.inbox-title')).toHaveText(INBOX_TITLE);
      assert.deepEqual(asked, [MAILBOX_ORIGIN + inboxLetterPath(letter.id)]);
    } finally {
      await alone.close();
    }
  } finally {
    await context.close();
  }
});

/** The invoice the live ride of the accounts of these checks produced, which is the ride just ended. */
function invoiceIdOfCurrentRide() {
  const id = sql(
    `SELECT invoice.id FROM invoices invoice
     JOIN rentals rental ON rental.id = invoice.rental_id
     WHERE rental.user_id IN (SELECT id FROM users WHERE email LIKE '${ACCOUNT_PREFIX}-%')`,
  );
  if (id === '') throw new Error('the ending issued no invoice');
  return id;
}

/** The total one invoice was issued at, as the exact string it publishes. */
function totalOf(invoiceId) {
  return sql(`SELECT total_amount_tyiyn FROM invoices WHERE id = '${invoiceId}'`);
}

/** Waits until the inbox holds the letter about one invoice, and answers what it holds. */
async function deliveredLetter(invoiceId) {
  return until(
    () => storedLetter(invoiceId),
    `no letter about the invoice ${invoiceId} was delivered`,
    DELIVERY_PATIENCE_MS,
  );
}
