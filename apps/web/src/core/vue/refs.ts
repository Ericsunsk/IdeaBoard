/** Structural ref contracts accept Vue refs and plain value holders. */
export type ValueRef<T> = {
  readonly value: T
}

export type MutableValueRef<T> = {
  value: T
}
