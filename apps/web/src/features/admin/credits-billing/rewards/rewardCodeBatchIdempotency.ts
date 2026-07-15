export function createRewardCodeBatchIdempotencyKey(): string {
  if (typeof crypto.randomUUID === 'function') {
    return `reward-batch:${crypto.randomUUID()}`
  }
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return `reward-batch:${Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')}`
}
