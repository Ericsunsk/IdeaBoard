export function withSignal(
  signal: AbortSignal | null | undefined,
): { signal: AbortSignal } | Record<string, never> {
  return signal == null ? {} : { signal }
}
