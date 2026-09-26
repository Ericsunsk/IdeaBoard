-- Encrypt outbound webhook signing secrets at rest (Security H1).
--
-- Before this migration event_subscriptions.signing_secret held the reversible
-- whsec_ transport string, so a database dump forged every subscriber webhook.
-- New columns record the storage envelope (following the
-- storage_profile_secret_versions material_kind/key_version pattern):
--
--   signing_secret_material_kind: 'plaintext' for pre-migration rows (accepted
--     on read only, converged by best-effort re-encrypt-on-read) and
--     'encrypted' for AES-256-GCM ciphertext over the whsec_ string
--     (APP_SECRETS_ENCRYPTION_KEY envelope, Stripe-credential precedent).
--   signing_secret_key_version: fingerprint of the encrypting master key
--     generation, so a rotation is detectable and fails closed.
--
-- ADD COLUMN with a DEFAULT backfills pre-existing rows as 'plaintext', which
-- the payload CHECK accepts only in whsec_ form; encrypted rows must be
-- non-blank. No data rewrite happens here: convergence is lazy on read.
ALTER TABLE event_subscriptions
  ADD COLUMN signing_secret_material_kind TEXT NOT NULL DEFAULT 'plaintext',
  ADD COLUMN signing_secret_key_version TEXT NOT NULL DEFAULT '';

ALTER TABLE event_subscriptions
  ADD CONSTRAINT chk_event_subscriptions_secret_material CHECK (signing_secret_material_kind IN ('plaintext', 'encrypted')),
  ADD CONSTRAINT chk_event_subscriptions_secret_payload CHECK (
    (signing_secret_material_kind = 'encrypted' AND btrim(signing_secret) <> '')
    OR (signing_secret_material_kind = 'plaintext' AND signing_secret LIKE 'whsec\_%')
  );
