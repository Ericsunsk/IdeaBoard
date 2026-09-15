import { useClipboard } from '@vueuse/core'

/**
 * Shared clipboard seam — returns boolean success so callers can route to
 * feature feedback boundaries without touching `navigator.clipboard` directly.
 *
 * NOTE: `@vueuse/core` `copy()` never rejects on failure and sets `copied`
 * unconditionally, so success must be derived from the native promise /
 * `execCommand` boolean here instead of assuming `await copy()` succeeded.
 */
export function useAppClipboard() {
  const { copy, copied, isSupported } = useClipboard({ legacy: true })

  async function copyText(text: string): Promise<boolean> {
    if (!text) return false
    // Fast path: async Clipboard API reports real success/failure.
    try {
      const clipboard = globalThis.navigator?.clipboard
      if (clipboard?.writeText) {
        await clipboard.writeText(text)
        // Sync vueuse `copied` animation state; ignore its result.
        await copy(text).catch(() => {})
        return true
      }
    } catch {
      // Fall through to legacy path below.
    }
    if (typeof document === 'undefined' || !isSupported.value) return false
    try {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      ta.setAttribute('readonly', '')
      document.body.appendChild(ta)
      ta.select()
      const ok = document.execCommand('copy')
      ta.remove()
      if (ok) await copy(text).catch(() => {})
      return ok
    } catch {
      return false
    }
  }

  return {
    copyText,
    copied,
    isSupported,
  }
}
