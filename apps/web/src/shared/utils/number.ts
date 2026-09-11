/** Numeric range-clamping primitives — no external deps. */

/**
 * Clamp `value` into the inclusive `[min, max]` range.
 *
 * Non-finite input propagates (`NaN` in → `NaN` out), matching the raw
 * `Math.max`/`Math.min` composition this replaces. Use
 * {@link clampWithFallback} when a non-finite value must resolve to a safe
 * default instead.
 */
export function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value))
}

/**
 * Clamp `value` into `[min, max]`, substituting `fallback` when the value is
 * not a finite number. The fallback is returned as-is (not itself clamped).
 */
export function clampWithFallback(
  value: number,
  min: number,
  max: number,
  fallback: number,
): number {
  if (!Number.isFinite(value)) return fallback
  return Math.max(min, Math.min(max, value))
}

/**
 * Clamp `value` into the unit range `[0, 1]`, substituting `fallback` (default
 * `0`) when the value is not a finite number.
 */
export function clampUnit(value: number, fallback = 0): number {
  return clampWithFallback(value, 0, 1, fallback)
}
