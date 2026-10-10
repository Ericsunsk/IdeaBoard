// Typed route-query readers for vue-router `route.query`.
//
// Query values are `string | null | (string | null)[]`: every inline
// `Array.isArray` guard collapses to its first entry here so call sites read
// `stringParam(route.query.x)` instead of re-implementing the unwrap.
// Repeated params (`?x=a&x=b`) resolve to the first value everywhere — most
// sites already did first-element; folder/workspace/accountTab/demo/benchmark
// previously treated any array as absent, now standardized on first-value.
//
// Type-only vue-router import: no runtime dependency on the router.
import type { LocationQueryValue } from 'vue-router'

export type RouteQueryParam = LocationQueryValue | LocationQueryValue[] | undefined

/** First string value of a route query param; non-strings and absence yield `undefined`. */
export function stringParam(value: RouteQueryParam): string | undefined {
  const first = Array.isArray(value) ? value[0] : value
  return typeof first === 'string' ? first : undefined
}

/** `stringParam` narrowed by a caller predicate; predicate failures yield `undefined`. */
export function stringParamWhere(
  value: RouteQueryParam,
  validate: (value: string) => boolean,
): string | undefined {
  const first = stringParam(value)
  return first !== undefined && validate(first) ? first : undefined
}
