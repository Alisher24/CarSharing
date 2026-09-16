/**
 * The locale every part of the interface writes numbers, prices and times in. It is one value
 * rather than a choice each view makes, so a price and a clock are never spelled two ways on one
 * screen.
 */
export const INTERFACE_LOCALE = 'ru-RU';

/**
 * The timezone the service states its days in. The backend owns this value — it is the row its
 * readiness operation publishes and the one its daily-limit statement reads — and a browser cannot
 * import it, so it is declared here, once, for every view that has to show a service day.
 */
export const SERVICE_TIME_ZONE = 'Asia/Bishkek';

/** The one way a moment of the contract is written, in the locale and the timezone above. */
const momentFormat = new Intl.DateTimeFormat(INTERFACE_LOCALE, {
  timeZone: SERVICE_TIME_ZONE,
  day: 'numeric',
  month: 'long',
  hour: '2-digit',
  minute: '2-digit',
});

/**
 * The same moment with the year it happened in. A history reaches back past the turn of a year, and
 * a day and a month alone would make two Septembers read alike.
 */
const datedMomentFormat = new Intl.DateTimeFormat(INTERFACE_LOCALE, {
  timeZone: SERVICE_TIME_ZONE,
  day: 'numeric',
  month: 'long',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
});

/**
 * serviceMoment writes a moment of the contract in the timezone the service states its days in, so
 * a moment that arrives as UTC is read as the local date and time a person acts on. A moment the
 * interface cannot read is left out rather than guessed at.
 */
export function serviceMoment(wireMoment: string): string | undefined {
  return formatted(momentFormat, wireMoment);
}

/** datedServiceMoment writes the same moment with its year, which is what a history is read by. */
export function datedServiceMoment(wireMoment: string): string | undefined {
  return formatted(datedMomentFormat, wireMoment);
}

function formatted(format: Intl.DateTimeFormat, wireMoment: string): string | undefined {
  const moment = Date.parse(wireMoment);
  if (Number.isNaN(moment)) return undefined;

  return format.format(moment);
}
