import type { InternalUserShape } from "./authUtils.js";

type QueryResult = {
  rows: unknown[];
  rowCount?: number | null;
};

type AuthIdentityDatabaseClient = {
  query(text: string, values?: unknown[]): Promise<QueryResult>;
  release(): void;
};

export type AuthIdentityDatabase = {
  query(text: string, values?: unknown[]): Promise<QueryResult>;
  connect(): Promise<AuthIdentityDatabaseClient>;
};

type UserRow = {
  id?: unknown;
  email?: unknown;
  name?: unknown;
  role?: unknown;
  emailVerified?: unknown;
};

export type AuthIdentityStore = {
  findUserByEmail(email: string): Promise<InternalUserShape | null>;
  updateAuthUserRole(
    authUserID: string,
    role: "admin" | "user",
  ): Promise<boolean>;
  finalizeTrustedRegistrationIdentity(
    authUserID: string,
    claimIdentity: string,
    role: "admin" | "user",
  ): Promise<boolean>;
  updateAuthUserBanState(
    authUserID: string,
    banned: boolean,
    banReason: string | null,
    banExpiresIn: number | null,
  ): Promise<boolean>;
  deleteAuthUserSessions(authUserID: string): Promise<boolean>;
  removeAuthUserCascade(authUserID: string): Promise<boolean>;
};

export function createAuthIdentityStore(
  database: AuthIdentityDatabase,
): AuthIdentityStore {
  return {
    async findUserByEmail(email) {
      const result = await database.query(
        'SELECT id, email, name, role, "emailVerified" FROM "user" WHERE email = $1 LIMIT 1',
        [email],
      );
      return result.rows[0]
        ? normalizeUserRow(result.rows[0] as UserRow)
        : null;
    },

    async updateAuthUserRole(authUserID, role) {
      const result = await database.query(
        'UPDATE "user" SET role = $1 WHERE id = $2',
        [role, authUserID],
      );
      return Number(result.rowCount || 0) > 0;
    },

    async finalizeTrustedRegistrationIdentity(
      authUserID,
      claimIdentity,
      role,
    ) {
      const result = await database.query(
        'UPDATE "user" SET "emailVerified" = TRUE, role = $1, "updatedAt" = $2 WHERE id = $3 AND email = $4',
        [role, new Date(), authUserID, claimIdentity],
      );
      return Number(result.rowCount || 0) > 0;
    },

    async updateAuthUserBanState(
      authUserID,
      banned,
      banReason,
      banExpiresIn,
    ) {
      const result = await database.query(
        'UPDATE "user" SET "banned" = $1, "banReason" = $2, "banExpires" = CASE WHEN $1 AND "banned" = TRUE THEN "banExpires" ELSE $3 END, "updatedAt" = $4 WHERE id = $5',
        [
          banned,
          banned ? banReason || "No reason" : null,
          banned && banExpiresIn
            ? new Date(Date.now() + banExpiresIn * 1_000)
            : null,
          new Date(),
          authUserID,
        ],
      );
      return Number(result.rowCount || 0) > 0;
    },

    async deleteAuthUserSessions(authUserID) {
      const result = await database.query(
        'SELECT id FROM "user" WHERE id = $1 LIMIT 1',
        [authUserID],
      );
      if (result.rows.length === 0) return false;
      await database.query('DELETE FROM "session" WHERE "userId" = $1', [
        authUserID,
      ]);
      return true;
    },

    async removeAuthUserCascade(authUserID) {
      const connection = await database.connect();
      try {
        await connection.query("BEGIN");
        const result = await connection.query(
          'SELECT id, email FROM "user" WHERE id = $1 LIMIT 1',
          [authUserID],
        );
        if (result.rows.length === 0) {
          await connection.query("ROLLBACK");
          return false;
        }

        const row = result.rows[0] as UserRow;
        const email = typeof row?.email === "string" ? row.email.trim() : "";
        await connection.query('DELETE FROM "session" WHERE "userId" = $1', [
          authUserID,
        ]);
        await connection.query('DELETE FROM "account" WHERE "userId" = $1', [
          authUserID,
        ]);
        const verificationIdentifiers = [email, authUserID].filter(Boolean);
        if (verificationIdentifiers.length > 0) {
          await connection.query(
            'DELETE FROM "verification" WHERE identifier = ANY($1::text[])',
            [verificationIdentifiers],
          );
        }
        await connection.query('DELETE FROM "user" WHERE id = $1', [
          authUserID,
        ]);
        await connection.query("COMMIT");
        return true;
      } catch (error) {
        try {
          await connection.query("ROLLBACK");
        } catch (rollbackError) {
          throw new AggregateError(
            [error, rollbackError],
            "Auth identity removal transaction failed",
          );
        }
        throw error;
      } finally {
        connection.release();
      }
    },
  };
}

function normalizeUserRow(row: UserRow): InternalUserShape | null {
  if (typeof row?.id !== "string" || !row.id.trim()) return null;
  return {
    id: row.id,
    email: typeof row.email === "string" ? row.email : null,
    name: typeof row.name === "string" ? row.name : null,
    role: typeof row.role === "string" ? row.role : null,
    emailVerified: row.emailVerified === true,
  };
}
