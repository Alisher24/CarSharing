import type { Submission } from './useAccount';

/**
 * Russian wording for each refusal the account operations can answer with, chosen by contract code
 * so that the text a person reads never depends on a message written for a developer.
 */
export const REFUSAL_TEXT: Record<string, string> = {
  EMAIL_ALREADY_REGISTERED: 'Этот адрес уже зарегистрирован. Войдите в существующий аккаунт.',
  INVALID_CREDENTIALS: 'Неверный адрес или пароль.',
  VALIDATION_FAILED: 'Проверьте адрес электронной почты и пароль.',
  ORIGIN_NOT_ALLOWED: 'Запрос отклонён. Откройте приложение по обычному адресу.',
  SERVICE_UNAVAILABLE: 'Сервис временно недоступен. Повторите попытку позже.',
};

export const UNREACHABLE_TEXT = 'Нет связи с сервисом. Проверьте подключение и повторите попытку.';

// A refusal the table does not name reached a working service that could not carry the operation
// out, which is a different thing from an unreachable service and must not be reported as one.
const UNEXPLAINED_REFUSAL_TEXT = 'Сервис не выполнил запрос. Повторите попытку позже.';

/** The sentence a refused or unreachable submission is explained with, or null when it succeeded. */
export function refusalText(submission: Submission): string | null {
  if (submission.state !== 'failed') return null;
  if (submission.result.outcome === 'refused') {
    return REFUSAL_TEXT[submission.result.code] ?? UNEXPLAINED_REFUSAL_TEXT;
  }
  if (submission.result.outcome === 'signed-out') return REFUSAL_TEXT.INVALID_CREDENTIALS;

  return UNREACHABLE_TEXT;
}
