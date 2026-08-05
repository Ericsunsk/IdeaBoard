import type { User } from '../../../shared/api/userSession'
import { clearStoredAuthState, getStoredUser, setStoredAuth } from '../../../shared/utils/auth'

export function readStoredSessionUser(): User | null {
  return getStoredUser()
}

export function persistSessionUser(user: User): void {
  setStoredAuth(user)
}

export function clearPersistedSession(): void {
  clearStoredAuthState()
}
