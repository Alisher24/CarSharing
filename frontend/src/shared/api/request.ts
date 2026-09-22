/**
 * How one request of the contract is made to this service, declared once for every wrapper above the
 * generated client. Nothing here reads or writes the session cookie: it is HttpOnly, so the browser
 * attaches it and the server replaces it.
 */

/** The credentials and caching every request of the service is made with. */
export const sameOriginRequest = { credentials: 'same-origin', cache: 'no-store' } as const;

/**
 * The origin the service demands of a changing request, which is the one this document is served
 * from. The contract declares `Origin` as a required header, so the generated types ask for it; a
 * browser sets the real one itself and refuses to let a script override it, so this value satisfies
 * the type while the value the server checks is the one the browser attached.
 */
export const originHeader = () => ({ Origin: window.location.origin });
