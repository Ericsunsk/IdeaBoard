/**
 * Shared clipboard seam — returns boolean success so callers can route to
 * feature feedback boundaries without touching `navigator.clipboard` directly.
 */
export function useAppClipboard() {
  async function copyText(text: string): Promise<boolean> {
    if (!text) return false

    const clipboard = globalThis.navigator?.clipboard
    if (!clipboard?.writeText) return false

    try {
      await clipboard.writeText(text)
      return true
    } catch {
      return false
    }
  }

  return { copyText }
}
