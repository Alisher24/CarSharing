/**
 * What each way of having nothing is called. A resource that has not answered yet must not be
 * announced as unavailable, and one that answered with nothing must not be announced as broken.
 */
export type AbsenceCopy = {
  loading: string;
  none: string;
  unreachable: string;
};
