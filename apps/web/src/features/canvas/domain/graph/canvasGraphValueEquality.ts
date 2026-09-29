/** Structural equality for JSON-shaped graph values. */
export function isSameJsonValue(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right)
}
