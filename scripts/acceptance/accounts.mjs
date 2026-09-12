// The account fields the acceptance suites assert about, in the form the service stores them. Both
// the password policy and the Argon2id parameters are documented values, so these are the single
// place the suites state them.
export const SHORTEST_ACCEPTED_PASSWORD = 12;
export const LONGEST_ACCEPTED_PASSWORD = 128;

export const PASSWORD_HASH_COLUMN = 'password_hash';
export const SESSION_TOKEN_COLUMN = 'token';

// A stored hash is a PHC string naming its algorithm and cost; the salt is its fifth $-separated
// field, and the service is expected to record Argon2id at v=19.
export const ARGON2ID_PHC_PATTERN = /^\$argon2id\$v=19\$m=(\d+),t=(\d+),p=(\d+)\$/;
export const PHC_SALT_FIELD = 5;
