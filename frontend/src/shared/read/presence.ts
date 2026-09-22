import type { Resource } from './Resource.ts';

/**
 * What a view found when it looked for the value it needs. The three ways of having nothing are kept
 * apart, because they are three different things to tell a person: the answer has not arrived yet,
 * the service answered and published none, or nothing could be read at all.
 */
export type Found<T> =
  { state: 'loading' } | { state: 'found'; value: T } | { state: 'none' } | { state: 'unreachable' };

/**
 * found is what one view has when it looks for a value: the value itself, or which of the three ways
 * of having nothing it met. It is stated once, so a map, a card and a feed of the cabinet all tell
 * an empty answer from an unreachable service the same way.
 */
export function found<T>(resource: Resource<unknown>, value: T | undefined): Found<T> {
  if (value !== undefined) return { state: 'found', value };
  if (resource.phase === 'loading') return { state: 'loading' };
  if (resource.phase === 'failed') return { state: 'unreachable' };
  return { state: 'none' };
}
