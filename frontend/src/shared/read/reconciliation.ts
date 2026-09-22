/**
 * The periodic reconciliation, and the one lever a test has over it.
 *
 * Reconciliation runs whenever the application is open, so a check of what the change stream
 * delivers has to be able to switch it off: with it running, a change would reach the screen through
 * a poll as well, and the check could not tell a delivered signal from a poll that found the change
 * on its own. The lever is a parameter of the address, it is read once per load, and it changes
 * nothing about the interval the application ships.
 */

/** How often every loaded resource is read again, which is what repairs a missed signal. */
export const RECONCILE_MILLISECONDS = 5_000;

/** The address parameter that switches reconciliation off, and the only value it answers to. */
const RECONCILE_PARAMETER = 'reconcile';
const WITHOUT_RECONCILIATION = 'off';

/**
 * reconciliationEnabled reports whether the periodic reads run for this page. It is read from the
 * address the application was loaded with, so the shipped interval is only ever changed by asking
 * for it explicitly.
 */
export function reconciliationEnabled(search: string = window.location.search): boolean {
  return new URLSearchParams(search).get(RECONCILE_PARAMETER) !== WITHOUT_RECONCILIATION;
}
