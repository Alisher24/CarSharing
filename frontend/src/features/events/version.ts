/**
 * Versions and the moments a snapshot was computed at are both compared as text. A version is a
 * decimal integer of up to nineteen digits, which is more than a JavaScript number holds exactly,
 * so it is never converted to one. A snapshot moment is a canonical timestamp whose shape the
 * contract fixes, so two of them order the same way as the instants they name.
 */

/** The version that precedes every published one: what a resource that has never been seen has. */
export const NO_VERSION = '0';

/** Whether one version is newer than another. */
export function isNewerVersion(compared: string, than: string): boolean {
  return compared !== than && hasMoreDigits(compared, than);
}

/** Which of two versions is newer, or zero when they are the same version. */
export function compareVersions(left: string, right: string): number {
  if (left === right) return 0;
  return isNewerVersion(left, right) ? 1 : -1;
}

/** The later of two versions. Both are canonical, so a longer one is always the larger. */
export function highestVersion(left: string, right: string): string {
  return compareVersions(left, right) >= 0 ? left : right;
}

/** Whether a snapshot computed at one moment is newer than one computed at another. */
export function isNewerTimestamp(compared: string, than: string): boolean {
  return compared > than;
}

// Leading zeros never appear in a canonical decimal, but a malformed one must not be read as the
// larger number merely because it is written with more digits.
function hasMoreDigits(left: string, right: string): boolean {
  const leftDigits = left.replace(/^0+/, '');
  const rightDigits = right.replace(/^0+/, '');
  if (leftDigits.length !== rightDigits.length) return leftDigits.length > rightDigits.length;
  return leftDigits > rightDigits;
}
