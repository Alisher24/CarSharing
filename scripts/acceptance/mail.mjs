// What the mail suites need beyond one HTTP call: the two addresses of the stub, the shape of a
// delivery key, the queries that read the mail schema, and the commands that stop, start and arm it.
//
// The inbox is published on the loopback address of the host, so a check reads a letter the way a
// person does. The internal listener is not published at all — that is the point of it — so a check
// that needs it sends its request from a container on the same network.
import { setTimeout as delay } from 'node:timers/promises';
import { MAILBOX_ORIGIN, asRole, compose, sql } from '../service.mjs';
import { call } from './client.mjs';

/** The paths of the mailbox, which is the surface a check reads from the host. */
export const MESSAGES_PATH = '/api/v1/messages';
export const messagePath = (id) => `${MESSAGES_PATH}/${id}`;

/** The paths of the internal listener, reached from inside the network rather than from the host. */
export const DELIVERY_PATH = '/internal/v1/messages';
export const MAIL_ACTION_PATH = '/internal/v1/demo/actions';

/** The role the mail stub connects as, and the role the application connects as. */
export const MAIL_ROLE = 'mailstub_app';
export const APPLICATION_ROLE = 'carsharing_app';

/** The kind of task an ending owes: the letter that carries its invoice. */
export const INVOICE_ISSUED_TASK = 'invoice.issued';

/** How long a check waits for the worker to deliver a letter the queue holds. */
const DELIVERY_PATIENCE_MS = 30_000;
const POLL_MS = 200;

/** The key one letter about one invoice is delivered under, as the contract declares its shape. */
export function deliveryKeyOf(invoiceId) {
  return `invoice:${invoiceId}:issued`;
}

/**
 * One request to the mailbox. The box is anonymous and read-only, so nothing here carries a
 * credential: what a check sends is the method, the path and the query a person would use.
 */
export async function mailbox(path, options = {}) {
  const response = await fetch(MAILBOX_ORIGIN + path, options);
  const text = await response.text();
  let json = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = null;
  }
  return { status: response.status, headers: response.headers, text, json };
}

/** Every letter of one page of the box, read the way a person reads it. */
export async function letters(query = '') {
  const answer = await mailbox(`${MESSAGES_PATH}${query}`);
  if (answer.status !== 200) throw new Error(answer.text);
  return answer.json;
}

/**
 * One request to the internal listener of the mail stub, sent from a container on the same network
 * because the listener is published nowhere. The status and the body are read from the answer, so a
 * check asserts about a refusal rather than about the exit code of a command.
 */
export function internalCall(method, path, { body, token, deliveryKey } = {}) {
  const args = ['-s', '-o', '-', '-w', '\n%{http_code}', '-X', method];
  if (body !== undefined) {
    args.push('-H', 'Content-Type: application/json', '--data-binary', body);
  }
  if (token !== undefined) {
    args.push('-H', `Authorization: Bearer ${token}`);
  }
  if (deliveryKey !== undefined) {
    args.push('-H', `Delivery-Key: ${deliveryKey}`);
  }
  const answer = compose(
    'run',
    '--rm',
    '--no-deps',
    '--entrypoint',
    'curl',
    'frontend',
    ...args,
    `http://mailstub:8080${path}`,
  );
  const lines = answer.split('\n');
  const status = Number(lines.at(-1));
  return { status, text: lines.slice(0, -1).join('\n') };
}

/**
 * Arms the loss of the next answer the stub produces, through the demonstration control the product
 * documents rather than by writing the table: the capability belongs to that command, and the process
 * that serves mail is not allowed to make the demand itself.
 */
export function armLostAnswer(actionId) {
  const args = ['--profile', 'demo', 'run', '--rm', 'democontrol', 'drop-next-response'];
  if (actionId !== undefined) args.push('--action-id', actionId);
  return JSON.parse(
    compose(...args)
      .split('\n')
      .filter((line) => line.trim() !== '')
      .at(-1),
  );
}

/** Stops the mail stub, which is how a check reaches a delivery that cannot be made. */
export function stopMailstub() {
  compose('stop', 'mailstub');
}

/** Starts the mail stub again and waits until its own probe reports it ready. */
export async function startMailstub() {
  compose('start', 'mailstub');
  await until(async () => {
    const answer = await mailbox(MESSAGES_PATH).catch(() => undefined);
    return answer?.status === 200;
  }, 'the mail stub never became ready again');
}

/** Starts the worker again, which is how a check lets the queue be delivered. */
export function startWorker() {
  compose('start', 'worker');
}

/**
 * How many letters the box holds about one invoice. The key is what the count is taken by rather than
 * the recipient: one letter about one invoice is the whole guarantee, and a second one under another
 * key would not be its repeat.
 */
export function lettersAbout(invoiceId) {
  return Number(sql(`SELECT count(*) FROM mailstub.messages WHERE delivery_key = '${deliveryKeyOf(invoiceId)}'`));
}

/** One stored letter about one invoice: what it says and the moment it was accepted at. */
export function storedLetter(invoiceId) {
  const row = sql(
    `SELECT id || '|' || recipient || '|' || subject || '|' ||
            to_char(accepted_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || '|' || body
     FROM mailstub.messages WHERE delivery_key = '${deliveryKeyOf(invoiceId)}'`,
  );
  if (row === '') return undefined;
  const [id, to, subject, acceptedAt, text] = row.split('|');
  return { id, to, subject, acceptedAt, text };
}

/** Every delivery key the box holds, which a check about a second letter reads. */
export function storedKeys() {
  const rows = sql('SELECT delivery_key FROM mailstub.messages ORDER BY accepted_at, id');
  return rows === '' ? [] : rows.split('\n');
}

/** Whether the stub is armed to lose the answer of its next delivery. */
export function armedFaults() {
  return Number(sql('SELECT count(*) FROM mailstub.demo_delivery_faults'));
}

/** The moment the armed demand was recorded, which the action's own answer states. */
export function armedAt() {
  return sql(
    `SELECT to_char(armed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
     FROM mailstub.demo_delivery_faults`,
  );
}

/** The demonstration actions the stub remembers, by the identifier they were asked under. */
export function rememberedActions() {
  const rows = sql('SELECT action_id FROM mailstub.demo_actions ORDER BY action_id');
  return rows === '' ? [] : rows.split('\n');
}

/**
 * The task the queue still owes for one invoice's letter, or undefined when it has none. A task that
 * was delivered is confirmed rather than deleted, so a check reads both what is outstanding and what
 * the attempt recorded before it succeeded.
 */
export function letterTask(invoiceId) {
  const row = sql(
    `SELECT attempts || '|' || coalesce(last_error, '') || '|' ||
            coalesce(to_char(next_attempt_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), '') || '|' ||
            coalesce(to_char(completed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), '')
     FROM outbox WHERE kind = '${INVOICE_ISSUED_TASK}' AND resource_id = '${invoiceId}'`,
  );
  if (row === '') return undefined;
  const [attempts, lastError, nextAttemptAt, completedAt] = row.split('|');
  return {
    attempts: Number(attempts),
    lastError,
    nextAttemptAt,
    completedAt: completedAt === '' ? undefined : completedAt,
  };
}

/** Whether the queue still owes the letter about one invoice. */
export function awaitingLetter(invoiceId) {
  return Number(
    sql(
      `SELECT count(*) FROM outbox
       WHERE kind = '${INVOICE_ISSUED_TASK}' AND resource_id = '${invoiceId}' AND completed_at IS NULL`,
    ),
  );
}

/** Runs one statement as one role and reports whether the database accepted it. */
export function asMailRole(statement) {
  return asRole(MAIL_ROLE, statement);
}

/** Runs one statement as the application's role and reports whether the database accepted it. */
export function asApplication(statement) {
  return asRole(APPLICATION_ROLE, statement);
}

/**
 * Waits until a letter about one invoice is in the box, and answers it. The worker delivers on its
 * own poll, so a check waits for the delivery rather than assuming how long an attempt took.
 */
export async function deliveredLetter(invoiceId) {
  return until(() => {
    const stored = storedLetter(invoiceId);
    return stored === undefined ? undefined : stored;
  }, `no letter about the invoice ${invoiceId} was delivered`);
}

/** Waits until `reached` answers something truthy, and fails with `complaint` when it never does. */
export async function until(reached, complaint, patienceMs = DELIVERY_PATIENCE_MS) {
  const deadline = Date.now() + patienceMs;
  for (;;) {
    const value = await reached();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`${complaint} within ${patienceMs} ms`);
    await delay(POLL_MS);
  }
}

/**
 * Removes everything this suite wrote: the letters about its own invoices, the demands and remembered
 * actions it armed, and then the rides and invoices themselves. The box belongs to the whole stack —
 * another suite's finished ride leaves a letter of its own — so only the keys this suite's accounts
 * produced are removed.
 */
export async function forgetSuiteMail(accountPrefix) {
  const mine = `(SELECT id FROM users WHERE email LIKE '${accountPrefix}-%')`;
  sql(
    `DELETE FROM mailstub.messages WHERE delivery_key IN (
       SELECT 'invoice:' || invoice.id || ':issued' FROM invoices invoice
       JOIN rentals rental ON rental.id = invoice.rental_id
       WHERE rental.user_id IN ${mine})`,
  );
  sql('DELETE FROM mailstub.demo_delivery_faults');
  sql('DELETE FROM mailstub.demo_actions');
  sql(
    `DELETE FROM outbox WHERE resource_id IN (
       SELECT invoice.id FROM invoices invoice
       JOIN rentals rental ON rental.id = invoice.rental_id
       WHERE rental.user_id IN ${mine})`,
  );
}

export { call, compose, sql };
