// The rate-limit settings acceptance suites use to predict the boundary of the running service.
// They are required rather than defaulted here: only the service owns a default, while a check must
// be told the same values Compose passed to that service.
const SETTINGS = [
  ['signInEmailAndAddress', 'RATE_LIMIT_SIGNIN_EMAIL_ADDRESS_ATTEMPTS'],
  ['signInEmail', 'RATE_LIMIT_SIGNIN_EMAIL_ATTEMPTS'],
  ['signInAddress', 'RATE_LIMIT_SIGNIN_ADDRESS_ATTEMPTS'],
  ['registrationAddress', 'RATE_LIMIT_REGISTRATION_ADDRESS_ATTEMPTS'],
];

export function rateLimitsFromEnvironment(environment) {
  const limits = {};
  for (const [property, variable] of SETTINGS) {
    const raw = environment[variable];
    if (raw === undefined || raw === '') throw new Error(`${variable} is required for acceptance`);

    const value = Number(raw);
    if (!Number.isInteger(value) || value <= 0) throw new Error(`${variable} must be a positive integer`);
    limits[property] = value;
  }
  return limits;
}
