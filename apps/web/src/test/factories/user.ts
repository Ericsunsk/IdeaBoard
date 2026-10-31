import type { User } from '@/api/http/sessionUserType'

export type TestUserOverrides = Partial<User> & {
  ordinal?: number
}

// Converged session-user factory: the ordinal-derived defaults come from
// `auth/session/authSession.contract.spec.ts` (the richest local variant);
// every other local `makeUser(): User` was a fixed subset of these fields.
export function makeSessionUser(overrides: TestUserOverrides = {}): User {
  const ordinal = overrides.ordinal ?? 1
  const { ordinal: _ignored, ...rest } = overrides
  return {
    public_id: `00000000-0000-4000-8000-${String(ordinal).padStart(12, '0')}`,
    display_name: `User ${ordinal}`,
    avatar: '',
    role: 'user',
    ...rest,
  }
}
