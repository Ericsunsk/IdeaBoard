import { authDB } from './auth.js'
import type { InternalUserShape } from './auth-utils.js'

type UserRow = {
  id?: unknown
  username?: unknown
  email?: unknown
  name?: unknown
  role?: unknown
}

export async function findUserByUsernameOrEmail(
  username: string,
  email: string,
): Promise<InternalUserShape | null> {
  const result = await authDB.query(
    'SELECT id, username, email, name, role FROM "user" WHERE username = $1 OR email = $2 LIMIT 1',
    [username, email],
  )
  if (result.rows.length === 0) {
    return null
  }
  return normalizeUserRow(result.rows[0])
}

export async function updateAuthUserRole(
  authUserID: string,
  role: 'admin' | 'user',
): Promise<boolean> {
  const result = await authDB.query('UPDATE "user" SET role = $1 WHERE id = $2', [role, authUserID])
  return Number(result.rowCount || 0) > 0
}

export async function updateAuthUserBanState(
  authUserID: string,
  banned: boolean,
  banReason: string | null,
  banExpiresIn: number | null,
): Promise<boolean> {
  const result = await authDB.query(
    'UPDATE "user" SET "banned" = $1, "banReason" = $2, "banExpires" = $3, "updatedAt" = $4 WHERE id = $5',
    [
      banned,
      banned ? banReason || 'No reason' : null,
      banned && banExpiresIn ? new Date(Date.now() + banExpiresIn * 1000) : null,
      new Date(),
      authUserID,
    ],
  )
  return Number(result.rowCount || 0) > 0
}

export async function deleteAuthUserSessions(authUserID: string): Promise<boolean> {
  const result = await authDB.query('SELECT id FROM "user" WHERE id = $1 LIMIT 1', [authUserID])
  if (result.rows.length === 0) {
    return false
  }
  await authDB.query('DELETE FROM "session" WHERE "userId" = $1', [authUserID])
  return true
}

export async function removeAuthUserCascade(authUserID: string): Promise<boolean> {
  const conn = await authDB.connect()
  try {
    await conn.query('BEGIN')
    const result = await conn.query('SELECT id, email, username FROM "user" WHERE id = $1 LIMIT 1', [
      authUserID,
    ])
    if (result.rows.length === 0) {
      await conn.query('ROLLBACK')
      return false
    }

    const row = result.rows[0] as UserRow
    const email = typeof row?.email === 'string' ? row.email.trim() : ''
    const username = typeof row?.username === 'string' ? row.username.trim() : ''

    await conn.query('DELETE FROM "session" WHERE "userId" = $1', [authUserID])
    await conn.query('DELETE FROM "account" WHERE "userId" = $1', [authUserID])

    const verificationIdentifiers = [email, username, authUserID].filter(Boolean)
    if (verificationIdentifiers.length > 0) {
      await conn.query('DELETE FROM "verification" WHERE identifier = ANY($1::text[])', [
        verificationIdentifiers,
      ])
    }

    await conn.query('DELETE FROM "user" WHERE id = $1', [authUserID])
    await conn.query('COMMIT')
    return true
  } catch (error) {
    await conn.query('ROLLBACK')
    throw error
  } finally {
    conn.release()
  }
}

function normalizeUserRow(row: UserRow): InternalUserShape | null {
  if (typeof row?.id !== 'string' || !row.id.trim()) {
    return null
  }
  return {
    id: row.id,
    username: typeof row.username === 'string' ? row.username : null,
    email: typeof row.email === 'string' ? row.email : null,
    name: typeof row.name === 'string' ? row.name : null,
    role: typeof row.role === 'string' ? row.role : null,
  }
}
