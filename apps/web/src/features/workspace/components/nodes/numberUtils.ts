export function toNumber(rawValue: string, fallback: number): number {
  const value = Number(rawValue)
  return Number.isFinite(value) ? value : fallback
}
