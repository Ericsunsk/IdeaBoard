-- Business ban deadline (issue #895). Mirrors
-- fragments/identity.sql: users.ban_expires_at is the deadline fact
-- source (NULL = permanent ban, never expires). The expiry sweep flips
-- an expired finite ban to active (status + Wallet) and persists an
-- unban Auth sync intent in one transaction, reusing the issue #888
-- per-user serialization and version fencing.
ALTER TABLE users
  ADD COLUMN ban_expires_at TIMESTAMPTZ(3) NULL;

CREATE INDEX idx_users_ban_expiry_sweep
  ON users (ban_expires_at, id)
  WHERE status = 0 AND ban_expires_at IS NOT NULL;
