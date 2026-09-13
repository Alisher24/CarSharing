import type { Found } from './useCatalog';

/**
 * What each way of having nothing is called. A resource that has not answered yet must not be
 * announced as unavailable, and one that answered with nothing must not be announced as broken.
 */
export type AbsenceCopy = {
  loading: string;
  none: string;
  unreachable: string;
};

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
        Повторить
      </button>
    </p>
  );
}
