import type { Pool } from 'pg'

const UNVERIFIED_USER_MAX_AGE_HOURS = 24

/** Remove abandoned sign-ups without touching verified identities or sessions. */
export async function cleanupUnverifiedUsers(pool: Pool): Promise<number> {
  const client = await pool.connect()
  try {
    await client.query('BEGIN')
    const result = await client.query<{ id: string }>(
      `
DELETE FROM "user"
WHERE "emailVerified" = FALSE
  AND "createdAt" < clock_timestamp() - ($1 * interval '1 hour')
RETURNING id
`,
      [UNVERIFIED_USER_MAX_AGE_HOURS],
    )
    await client.query('COMMIT')
    return result.rows.length
  } catch (error) {
    try {
      await client.query('ROLLBACK')
    } catch (rollbackError) {
      throw new AggregateError([error, rollbackError], 'unverified user cleanup transaction failed')
    }
    throw error
  } finally {
    client.release()
  }
}

export const AUTH_CLEANUP_INTERVAL_MS = 60 * 60 * 1000
