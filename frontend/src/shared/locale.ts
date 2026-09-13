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
