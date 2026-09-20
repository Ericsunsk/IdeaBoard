const crockfordAlphabet = '0123456789ABCDEFGHJKMNPQRSTVWXYZ'

export function generateRewardCode(): string {
  const bytes = new Uint8Array(12)
  crypto.getRandomValues(bytes)
  const payload = Array.from(
    bytes,
    (value) => crockfordAlphabet[value % crockfordAlphabet.length],
  ).join('')
  return `${payload.slice(0, 4)}-${payload.slice(4, 8)}-${payload.slice(8, 12)}`
}
