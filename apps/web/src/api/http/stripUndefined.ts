/**
 * Strips own enumerable properties whose value is `undefined`.
 *
 * Use it only at transport boundaries (generated DTO params/bodies) where the
 * target type cannot be widened. Own option/view-model/domain seams must say
 * `| undefined` explicitly instead so the contract stays visible.
 */
export type DefinedOnly<T extends object> = {
  [K in keyof T as undefined extends T[K] ? never : K]: T[K]
} & {
  [K in keyof T as undefined extends T[K] ? K : never]?: Exclude<T[K], undefined>
}

export function stripUndefined<const T extends object>(input: T): DefinedOnly<T> {
  return Object.fromEntries(
    Object.entries(input).filter(([, value]) => value !== undefined),
  ) as DefinedOnly<T>
}
