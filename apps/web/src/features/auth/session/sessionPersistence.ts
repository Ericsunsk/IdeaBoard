import type { User } from '@/api/http/sessionUserType'
import { clearStoredAuth, getStoredUser, setStoredAuth } from '../../../core/utils/auth'

export function readStoredSessionUser(): User | null {
  return getStoredUser()
}

export function persistSessionUser(user: User): void {
  setStoredAuth(user)
}

export function clearPersistedSession(): void {
  clearStoredAuth()
}
