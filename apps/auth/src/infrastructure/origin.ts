export function normalizeOrigin(raw: string): string {
  try {
    const parsed = new URL(raw.trim())
    const protocol = parsed.protocol.toLowerCase()
    if (
      (protocol !== 'http:' && protocol !== 'https:') ||
      (parsed.pathname !== '' && parsed.pathname !== '/') ||
      parsed.search ||
      parsed.hash ||
      parsed.username ||
      parsed.password ||
      !parsed.hostname
    ) {
      return ''
    }
    const hostname = parsed.hostname.toLowerCase()
    const port =
      parsed.port &&
      !(
        (protocol === 'http:' && parsed.port === '80') ||
        (protocol === 'https:' && parsed.port === '443')
      )
        ? `:${parsed.port}`
        : ''
    return `${protocol}//${hostname}${port}`
  } catch {
    return ''
  }
}
