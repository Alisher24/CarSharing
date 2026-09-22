import { RETRY_ACTION } from '../copy.ts';
import type { Found } from '../read/presence.ts';
import type { AbsenceCopy } from './absence.ts';

type ResourceNoticeProps = {
  found: Found<unknown>;
  copy: AbsenceCopy;
  onRetry: () => void;
};

/**
 * ResourceNotice explains why one resource has nothing to show, and offers another attempt where
 * another attempt could help. A resource that did answer needs no notice: the value is the answer.
 */
export function ResourceNotice({ found, copy, onRetry }: ResourceNoticeProps) {
  if (found.state === 'found') return null;
  if (found.state === 'loading') return <p className="resource-notice">{copy.loading}</p>;

  // The service published nothing, which another attempt would publish again.
  if (found.state === 'none') {
    return (
      <p className="resource-notice resource-notice-warning" role="status">
        {copy.none}
      </p>
    );
  }

  return (
    <p className="resource-notice resource-notice-warning" role="status">
      {copy.unreachable}
      <button className="fleet-retry" type="button" onClick={onRetry}>
        {RETRY_ACTION}
      </button>
    </p>
  );
}
