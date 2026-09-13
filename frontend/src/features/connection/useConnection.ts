import { getHealth, type ReadyStatus } from '../../shared/api/health';
import type { Resource } from '../../shared/api/Resource';
import { useResource } from '../../shared/api/useResource';

/**
 * How often the browser checks that it can still reach the service. This is the reader's own link,
 * shown in the header; a vehicle's link to the platform is a separate fact reported on its card.
 */
const CONNECTION_CHECK_MILLISECONDS = 10_000;

/** What the interface knows about the service behind it. */
export type Connection = Resource<ReadyStatus>;

export function useConnection(): Connection {
  return useResource(getHealth, CONNECTION_CHECK_MILLISECONDS).resource;
}
