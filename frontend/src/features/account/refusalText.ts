import { refusalText as sharedRefusalText } from '../../shared/refusal.ts';
import type { Submission } from './useAccount.ts';

/**
 * What a failed submission of the entry window is explained with, chosen by the contract code the
 * server answered with. The sentences themselves belong to the interface rather than to this window:
 * the same code is answered by a command of the account's own rental, and a person must read the same
 * thing about it wherever they met it.
 */

/** What a submission the service could not be reached for is reported as. */
export const UNREACHABLE_TEXT = 'Нет связи с сервисом. Проверьте подключение и повторите попытку.';

/** The sentence a refused or unreachable submission is explained with, or null when it succeeded. */
export function refusalText(submission: Submission): string | null {
  if (submission.state !== 'failed') return null;
  if (submission.result.outcome === 'refused') return sharedRefusalText(submission.result.code);
  if (submission.result.outcome === 'signed-out') return sharedRefusalText('INVALID_CREDENTIALS');

  return UNREACHABLE_TEXT;
}
