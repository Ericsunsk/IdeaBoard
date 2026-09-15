/**
 * Shared browser-storage seam (imperative).
 *
 * Centralizes the `typeof window` guard + quota/JSON `try/catch` that was
 * previously copy-pasted across board catalog, canvas viewport and search
 * persistence. For new *reactive* state prefer `@vueuse/core`
 * `useLocalStorage` / `useSessionStorage` instead of adding helpers here.
 */

export function readLocalStorageItem(
  key: string,
  onError?: (error: unknown) => void,
): string | null {
  if (typeof window === 'undefined' || typeof window.localStorage === 'undefined') return null
  try {
    return window.localStorage.getItem(key)
  } catch (error) {
    onError?.(error)
    return null
  }
}

export function writeLocalStorageItem(
  key: string,
  value: string,
  onError?: (error: unknown) => void,
): boolean {
  if (typeof window === 'undefined' || typeof window.localStorage === 'undefined') return false
  try {
    window.localStorage.setItem(key, value)
    return true
  } catch (error) {
    onError?.(error)
    return false
  }
}

export function readLocalStorageJson<T>(
  key: string,
  validate: (value: unknown) => T | null,
  onError?: (error: unknown) => void,
): T | null {
  const raw = readLocalStorageItem(key, onError)
  if (!raw) return null
  try {
    return validate(JSON.parse(raw))
  } catch (error) {
    onError?.(error)
    return null
  }
}

export function writeLocalStorageJson(
  key: string,
  value: unknown,
  onError?: (error: unknown) => void,
): boolean {
  let raw: string
  try {
    raw = JSON.stringify(value)
  } catch (error) {
    onError?.(error)
    return false
  }
  return writeLocalStorageItem(key, raw, onError)
}

export function removeLocalStorageItem(key: string, onError?: (error: unknown) => void): void {
  if (typeof window === 'undefined' || typeof window.localStorage === 'undefined') return
  try {
    window.localStorage.removeItem(key)
  } catch (error) {
    onError?.(error)
  }
}

function sessionStorageOrNull(): Storage | null {
  if (typeof window === 'undefined' || typeof window.sessionStorage === 'undefined') return null
  return window.sessionStorage
}

export function readSessionStorageItem(
  key: string,
  onError?: (error: unknown) => void,
): string | null {
  const storage = sessionStorageOrNull()
  if (!storage) return null
  try {
    return storage.getItem(key)
  } catch (error) {
    onError?.(error)
    return null
  }
}

export function writeSessionStorageItem(
  key: string,
  value: string,
  onError?: (error: unknown) => void,
): boolean {
  const storage = sessionStorageOrNull()
  if (!storage) return false
  try {
    storage.setItem(key, value)
    return true
  } catch (error) {
    onError?.(error)
    return false
  }
}

export function removeSessionStorageItem(key: string, onError?: (error: unknown) => void): void {
  const storage = sessionStorageOrNull()
  if (!storage) return
  try {
    storage.removeItem(key)
  } catch (error) {
    onError?.(error)
  }
}
