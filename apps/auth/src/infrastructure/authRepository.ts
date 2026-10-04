import type { InternalUserShape } from './authUtils.js'

type QueryResult = {
  rows: unknown[]
  rowCount?: number | null
}

type AuthIdentityDatabaseClient = {
  query(text: string, values?: unknown[]): Promise<QueryResult>
  release(): void
}

export type AuthIdentityDatabase = {
  query(text: string, values?: unknown[]): Promise<QueryResult>
  connect(): Promise<AuthIdentityDatabaseClient>
}

type UserRow = {
  id?: unknown
  email?: unknown
  name?: unknown
  role?: unknown
  emailVerified?: unknown
  providers?: unknown
}

export type AuthIdentityStore = {
  findUserByEmail(email: string): Promise<InternalUserShape | null>
  findUserEmailsByIDs(
    authUserIDs: string[],
  ): Promise<Array<{ auth_user_id: string; email: string; providers: string[] }>>
  findUserStatesByIDs(authUserIDs: string[]): Promise<
    Array<{
      auth_user_id: string
      role: string
      banned: boolean
      ban_reason: string | null
      ban_expires: string | null
    }>
  >
  updateAuthUserRole(authUserID: string, role: 'admin' | 'user'): Promise<boolean>
  finalizeTrustedRegistrationIdentity(
    authUserID: string,
    claimIdentity: string,
    role: 'admin' | 'user',
  ): Promise<boolean>
  updateAuthUserBanState(
    authUserID: string,
    banned: boolean,
    banReason: string | null,
    banExpiresIn: number | null,
  ): Promise<boolean>
  deleteAuthUserSessions(authUserID: string): Promise<boolean>
  removeAuthUserCascade(authUserID: string): Promise<boolean>
}

export function createAuthIdentityStore(database: AuthIdentityDatabase): AuthIdentityStore {
  return {
    async findUserByEmail(email) {
      const result = await database.query(
        'SELECT id, email, name, role, "emailVerified" FROM "user" WHERE email = $1 LIMIT 1',
        [email],
      )
      return result.rows[0] ? normalizeUserRow(result.rows[0] as UserRow) : null
    },

    async findUserEmailsByIDs(authUserIDs) {
      const result = await database.query(
        `
SELECT users.id,
       users.email,
       COALESCE(
         ARRAY_AGG(
           DISTINCT CASE
             WHEN accounts."providerId" = 'credential' THEN 'email'
             ELSE accounts."providerId"
           END
           ORDER BY CASE
             WHEN accounts."providerId" = 'credential' THEN 'email'
             ELSE accounts."providerId"
           END
         ) FILTER (WHERE accounts."providerId" IS NOT NULL),
         ARRAY['email']::text[]
       ) AS providers
FROM "user" users
LEFT JOIN "account" accounts ON accounts."userId" = users.id
WHERE users.id = ANY($1::text[])
GROUP BY users.id, users.email`,
        [authUserIDs],
      )
      return result.rows.flatMap((row) => {
        const candidate = row as UserRow
        const providers = Array.isArray(candidate.providers)
          ? candidate.providers.filter(
              (provider): provider is string => typeof provider === 'string' && provider.length > 0,
            )
          : []
        if (
          typeof candidate.id !== 'string' ||
          typeof candidate.email !== 'string' ||
          providers.length === 0
        )
          return []
        return [
          {
            auth_user_id: candidate.id,
            email: candidate.email,
            providers,
          },
        ]
      })
    },

    async findUserStatesByIDs(authUserIDs) {
      const result = await database.query(
        `
SELECT users.id,
       users.role,
       users.banned,
       users."banReason" AS "banReason",
       users."banExpires" AS "banExpires"
FROM "user" users
WHERE users.id = ANY($1::text[])`,
        [authUserIDs],
      )
      return result.rows.flatMap((row) => {
        const candidate = row as UserRow & {
          banned?: unknown
          banReason?: unknown
          banExpires?: unknown
        }
        if (typeof candidate.id !== 'string' || candidate.id.length === 0) return []
        // Pass the stored role through verbatim: an unexpected Auth-side
        // role is itself drift the reconciler must report as a role
        // mismatch, not as a missing identity.
        const role = typeof candidate.role === 'string' ? candidate.role : ''
        if (role.length === 0) return []
        const banExpires =
          candidate.banExpires instanceof Date
            ? candidate.banExpires.toISOString()
            : typeof candidate.banExpires === 'string' && candidate.banExpires.length > 0
              ? candidate.banExpires
              : null
        return [
          {
            auth_user_id: candidate.id,
            role,
            banned: candidate.banned === true,
            ban_reason: typeof candidate.banReason === 'string' ? candidate.banReason : null,
            ban_expires: banExpires,
          },
        ]
      })
    },

    async updateAuthUserRole(authUserID, role) {
      const result = await database.query('UPDATE "user" SET role = $1 WHERE id = $2', [
        role,
        authUserID,
      ])
      return Number(result.rowCount || 0) > 0
    },

    async finalizeTrustedRegistrationIdentity(authUserID, claimIdentity, role) {
      const result = await database.query(
        'UPDATE "user" SET "emailVerified" = TRUE, role = $1, "updatedAt" = $2 WHERE id = $3 AND email = $4',
        [role, new Date(), authUserID, claimIdentity],
      )
      return Number(result.rowCount || 0) > 0
    },

    async updateAuthUserBanState(authUserID, banned, banReason, banExpiresIn) {
      const result = await database.query(
        'UPDATE "user" SET "banned" = $1, "banReason" = $2, "banExpires" = CASE WHEN $1 AND "banned" = TRUE THEN "banExpires" ELSE $3 END, "updatedAt" = $4 WHERE id = $5',
        [
          banned,
          banned ? banReason || 'No reason' : null,
          banned && banExpiresIn ? new Date(Date.now() + banExpiresIn * 1_000) : null,
          new Date(),
          authUserID,
        ],
      )
      return Number(result.rowCount || 0) > 0
    },

    async deleteAuthUserSessions(authUserID) {
      const result = await database.query('SELECT id FROM "user" WHERE id = $1 LIMIT 1', [
        authUserID,
      ])
      if (result.rows.length === 0) return false
      await database.query('DELETE FROM "session" WHERE "userId" = $1', [authUserID])
      return true
    },

    async removeAuthUserCascade(authUserID) {
      const connection = await database.connect()
      try {
        await connection.query('BEGIN')
        const result = await connection.query(
          'SELECT id, email FROM "user" WHERE id = $1 LIMIT 1',
          [authUserID],
        )
        if (result.rows.length === 0) {
          await connection.query('ROLLBACK')
          return false
        }

        const row = result.rows[0] as UserRow
        const email = typeof row?.email === 'string' ? row.email.trim() : ''
        await connection.query('DELETE FROM "session" WHERE "userId" = $1', [authUserID])
        await connection.query('DELETE FROM "account" WHERE "userId" = $1', [authUserID])
        const verificationIdentifiers = [email, authUserID].filter(Boolean)
        if (verificationIdentifiers.length > 0) {
          await connection.query('DELETE FROM "verification" WHERE identifier = ANY($1::text[])', [
            verificationIdentifiers,
          ])
        }
        await connection.query('DELETE FROM "user" WHERE id = $1', [authUserID])
        await connection.query('COMMIT')
        return true
      } catch (error) {
        try {
          await connection.query('ROLLBACK')
        } catch (rollbackError) {
          throw new AggregateError(
            [error, rollbackError],
            'Auth identity removal transaction failed',
          )
        }
        throw error
      } finally {
        connection.release()
      }
    },
  }
}

function normalizeUserRow(row: UserRow): InternalUserShape | null {
  if (typeof row?.id !== 'string' || !row.id.trim()) return null
  return {
    id: row.id,
    email: typeof row.email === 'string' ? row.email : null,
    name: typeof row.name === 'string' ? row.name : null,
    role: typeof row.role === 'string' ? row.role : null,
    emailVerified: row.emailVerified === true,
  }
}
