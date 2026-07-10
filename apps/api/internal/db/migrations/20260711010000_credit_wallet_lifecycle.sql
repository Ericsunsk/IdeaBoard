-- migrate:up
CREATE TABLE credit_welcome_claims (
  identity_fingerprint CHAR(64) PRIMARY KEY,
  claimed_user_id BIGINT NOT NULL UNIQUE,
  claimed_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  CONSTRAINT fk_credit_welcome_claims_user_id
    FOREIGN KEY (claimed_user_id) REFERENCES users(id) ON DELETE RESTRICT,
  CONSTRAINT ck_credit_welcome_claims_fingerprint
    CHECK (identity_fingerprint ~ '^[0-9a-f]{64}$')
);

CREATE TABLE credit_welcome_identity_key_guard (
  singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
  key_fingerprint CHAR(64) NOT NULL,
  bound_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  CONSTRAINT ck_credit_welcome_identity_key_fingerprint
    CHECK (key_fingerprint ~ '^[0-9a-f]{64}$')
);

CREATE TABLE user_deletion_requests (
  user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
  auth_user_id VARCHAR(191),
  storage_uid VARCHAR(6),
  phase VARCHAR(32) NOT NULL DEFAULT 'pending_auth',
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ(3),
  CONSTRAINT ck_user_deletion_requests_phase
    CHECK (phase IN ('pending_auth', 'pending_storage', 'pending_tombstone', 'completed'))
);

-- statement:begin
CREATE OR REPLACE FUNCTION bind_credit_welcome_identity_key(p_key_fingerprint TEXT)
RETURNS VOID AS $$
DECLARE
  v_bound_fingerprint TEXT;
BEGIN
  p_key_fingerprint := btrim(p_key_fingerprint);
  IF p_key_fingerprint !~ '^[0-9a-f]{64}$' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid Welcome identity key fingerprint';
  END IF;

  PERFORM pg_advisory_xact_lock(5917936940700615137);
  SELECT key_fingerprint INTO v_bound_fingerprint
  FROM credit_welcome_identity_key_guard
  WHERE singleton = TRUE;

  IF NOT FOUND THEN
    INSERT INTO credit_welcome_identity_key_guard (singleton, key_fingerprint)
    VALUES (TRUE, p_key_fingerprint);
    RETURN;
  END IF;

  IF v_bound_fingerprint <> p_key_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE = 'IB010', MESSAGE = 'Welcome identity key does not match the permanently bound key';
  END IF;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE OR REPLACE FUNCTION claim_credit_welcome(p_identity_fingerprint TEXT, p_user_id BIGINT)
RETURNS BOOLEAN AS $$
BEGIN
  p_identity_fingerprint := btrim(p_identity_fingerprint);
  IF p_identity_fingerprint !~ '^[0-9a-f]{64}$'
     OR p_user_id <= 0 OR NOT EXISTS (SELECT 1 FROM users WHERE id = p_user_id) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid Credit Welcome claim';
  END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(p_identity_fingerprint, 8163442859039458091));
  IF EXISTS (
    SELECT 1 FROM credit_welcome_claims
    WHERE identity_fingerprint = p_identity_fingerprint OR claimed_user_id = p_user_id
  ) THEN
    RETURN FALSE;
  END IF;
  INSERT INTO credit_welcome_claims (identity_fingerprint, claimed_user_id)
  VALUES (p_identity_fingerprint, p_user_id);
  RETURN TRUE;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE OR REPLACE FUNCTION provision_credit_wallet(p_user_id BIGINT)
RETURNS VOID AS $$
DECLARE
  v_shard_key VARCHAR(128);
BEGIN
  IF p_user_id <= 0 OR NOT EXISTS (SELECT 1 FROM users WHERE id = p_user_id) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit wallet owner';
  END IF;

  v_shard_key := 'wallet:' || p_user_id::TEXT;
  INSERT INTO credit_wallets (user_id) VALUES (p_user_id) ON CONFLICT DO NOTHING;
  INSERT INTO credit_accounts (wallet_user_id, account_role)
  VALUES (p_user_id, 'user_available'), (p_user_id, 'user_reserved')
  ON CONFLICT DO NOTHING;
  INSERT INTO credit_accounts (account_role, shard_key, metadata)
  VALUES
    ('system_issuance', v_shard_key, jsonb_build_object('scope', 'wallet', 'wallet_user_id', p_user_id)),
    ('system_revenue', v_shard_key, jsonb_build_object('scope', 'wallet', 'wallet_user_id', p_user_id)),
    ('system_sink', v_shard_key, jsonb_build_object('scope', 'wallet', 'wallet_user_id', p_user_id))
  ON CONFLICT DO NOTHING;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
CREATE OR REPLACE FUNCTION set_credit_wallet_status(p_user_id BIGINT, p_status TEXT)
RETURNS VOID AS $$
BEGIN
  p_status := btrim(p_status);
  IF p_user_id <= 0 OR p_status NOT IN ('active', 'frozen') THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit wallet status command';
  END IF;
  UPDATE credit_wallets SET status = p_status, updated_at = now() WHERE user_id = p_user_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;
-- statement:end

-- statement:begin
DO $$
DECLARE
  trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.bind_credit_welcome_identity_key(TEXT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
  EXECUTE format(
    'ALTER FUNCTION %I.claim_credit_welcome(TEXT, BIGINT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
  EXECUTE format(
    'ALTER FUNCTION %I.provision_credit_wallet(BIGINT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
  EXECUTE format(
    'ALTER FUNCTION %I.set_credit_wallet_status(BIGINT, TEXT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
END;
$$;
-- statement:end

REVOKE ALL ON FUNCTION provision_credit_wallet(BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION set_credit_wallet_status(BIGINT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION claim_credit_welcome(TEXT, BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION bind_credit_welcome_identity_key(TEXT) FROM PUBLIC;

-- migrate:down
DROP FUNCTION IF EXISTS set_credit_wallet_status(BIGINT, TEXT);
DROP FUNCTION IF EXISTS provision_credit_wallet(BIGINT);
DROP FUNCTION IF EXISTS claim_credit_welcome(TEXT, BIGINT);
DROP FUNCTION IF EXISTS bind_credit_welcome_identity_key(TEXT);
DROP TABLE IF EXISTS user_deletion_requests;
DROP TABLE IF EXISTS credit_welcome_claims;
DROP TABLE IF EXISTS credit_welcome_identity_key_guard;
