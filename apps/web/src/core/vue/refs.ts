/** Structural ref contracts accept Vue refs and plain value holders. */
export type ValueRef<T> = {
  readonly value: T
}

export type MutableValueRef<T> = {
  value: T
}

/** Template refs may expose a DOM element directly or through a component root. */
export type ElementRefTarget = Element | { $el?: unknown } | null
