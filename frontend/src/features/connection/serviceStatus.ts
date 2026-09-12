/** What every value the connection card cannot read is shown as. */
export const EMPTY_VALUE = '—';

/** The locale the interface writes server times in. */
export const INTERFACE_LOCALE = 'ru-RU';

/** How each currency the service can report is written out. An unknown one is shown as it arrives. */
export const CURRENCY_LABELS: Record<string, string> = {
  KGS: 'Кыргызский сом · KGS',
};

export function currencyLabel(currency: string): string {
  return CURRENCY_LABELS[currency] ?? currency;
}

/**
 * formatCheckedAt renders the server time in the zone the server reported for itself, under the
 * city it reported for itself, so the interface never states a city and a clock that disagree.
 */
export function formatCheckedAt(serverTime: string, timeZone: string, city: string): string {
  const checkedAt = new Intl.DateTimeFormat(INTERFACE_LOCALE, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    timeZone,
  }).format(new Date(serverTime));

  return `${checkedAt} · ${city}`;
}
