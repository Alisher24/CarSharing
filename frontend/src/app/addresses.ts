/**
 * The addresses of the application. They are part of what a person can send to somebody else and of
 * what a reload has to bring back, so they are declared once here and named by every link, every
 * route and every redirect rather than written out where they are used.
 */

/** The map and the rental in force, which is what the application opens on. */
export const MAP_ADDRESS = '/';

/** The cabinet itself, which shows the rides: an address without a feed leads to that one. */
export const ACCOUNT_ADDRESS = '/account';

/** The feed of the rides the account has finished. */
export const RIDES_ADDRESS = '/account/rides';

/** The feed of the account's invoices. */
export const INVOICES_ADDRESS = '/account/invoices';

/** The segment one invoice is named by, which the route reads the identifier of an invoice from. */
export const INVOICE_PARAMETER = 'invoiceId';

/** The address of one invoice of the account. */
export function invoiceAddress(invoiceId: string): string {
  return `${INVOICES_ADDRESS}/${encodeURIComponent(invoiceId)}`;
}

/** Which feed of the cabinet an address belongs to, or nothing when it is not in the cabinet. */
export type CabinetSection = 'rides' | 'invoices';

/**
 * sectionOf reads which feed of the cabinet an address is in, so the tab of that feed is the one
 * marked as current. One invoice is part of the invoices feed rather than an address of its own:
 * the tab a person came through stays marked while they read the invoice it led to.
 */
export function sectionOf(pathname: string): CabinetSection | undefined {
  if (pathname === RIDES_ADDRESS) return 'rides';
  if (pathname === INVOICES_ADDRESS || pathname.startsWith(`${INVOICES_ADDRESS}/`)) return 'invoices';

  return undefined;
}
