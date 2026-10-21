-- Billing Account Wallet ownership (#916).
--
-- Wallet 唯一归属 Billing Account: credit_wallets 的主键从 user_id 切换为
-- billing_account_id,全部 Credit Commands / Restrictions / lifecycle /
-- Reconciliation 使用统一账户契约,Actor 记录真实调用身份。
--
-- 存量确定性归属:既有个人余额、历史交易、未完 Hold 全部归入该用户默认个人
-- 空间的 per-workspace 账户(每人初始个人管理账户),不复制、不合并、不重写
-- immutable Ledger 事实(金额/postings 不动,只回填新增的归属列);个人资金不
-- 注入组织(组织账户永远不是归属目标,新组织/共享账户从 0 开始)。
--
-- destructive: approved
-- 本文件包含评审后破坏性 DDL(PK 切换、失效 FK/CHECK/INDEX 重建、被取代的
-- user-keyed 函数下线),需评审通过后以 ATLAS_ALLOW_DESTRUCTIVE=1 执行;
-- 不含 SET NOT NULL / ADD COLUMN NOT NULL(门禁无豁免):新列一律 nullable
-- 加入、回填、再用 CHECK 约束收敛,与 fragments 快照保持一致。

-- 1. 归属解析函数(幂等,供回填与运行时复用)。
CREATE FUNCTION ensure_personal_billing_account_for_user(p_user_id BIGINT)
RETURNS BIGINT
LANGUAGE plpgsql SECURITY DEFINER
SET search_path TO 'public', 'pg_catalog', 'pg_temp'
AS $$
DECLARE
  v_workspace_id BIGINT;
  v_billing_account_id BIGINT;
BEGIN
  IF p_user_id <= 0 OR NOT EXISTS (SELECT 1 FROM users WHERE id = p_user_id) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid personal billing account owner';
  END IF;

  SELECT internal_id INTO v_workspace_id
  FROM workspaces
  WHERE owner_user_id = p_user_id AND organization_id IS NULL AND is_default
  LIMIT 1;
  IF NOT FOUND THEN
    FOR _ IN 1..8 LOOP
      BEGIN
        INSERT INTO workspaces (public_id, owner_user_id, name, is_default)
        VALUES ('ws_' || substr(md5(gen_random_uuid()::TEXT), 1, 12), p_user_id, 'My Workspace', TRUE)
        RETURNING workspaces.internal_id INTO v_workspace_id;
        EXIT;
      EXCEPTION WHEN unique_violation THEN
        SELECT internal_id INTO v_workspace_id
        FROM workspaces
        WHERE owner_user_id = p_user_id AND organization_id IS NULL AND is_default
        LIMIT 1;
        IF FOUND THEN
          EXIT;
        END IF;
      END;
    END LOOP;
    IF v_workspace_id IS NULL THEN
      RAISE EXCEPTION 'personal default workspace could not be provisioned';
    END IF;
  END IF;

  INSERT INTO workspace_memberships (workspace_id, user_id, role)
  VALUES (v_workspace_id, p_user_id, 'owner')
  ON CONFLICT (workspace_id, user_id) DO NOTHING;

  SELECT internal_id INTO v_billing_account_id
  FROM billing_accounts
  WHERE workspace_id = v_workspace_id
  LIMIT 1;
  IF NOT FOUND THEN
    FOR _ IN 1..8 LOOP
      BEGIN
        INSERT INTO billing_accounts (public_id, workspace_id, organization_id)
        VALUES ('ba_' || substr(md5(gen_random_uuid()::TEXT), 1, 12), v_workspace_id, NULL)
        RETURNING billing_accounts.internal_id INTO v_billing_account_id;
        EXIT;
      EXCEPTION WHEN unique_violation THEN
        SELECT internal_id INTO v_billing_account_id
        FROM billing_accounts
        WHERE workspace_id = v_workspace_id
        LIMIT 1;
        IF FOUND THEN
          EXIT;
        END IF;
      END;
    END LOOP;
    IF v_billing_account_id IS NULL THEN
      RAISE EXCEPTION 'personal billing account could not be provisioned';
    END IF;
  END IF;

  INSERT INTO billing_account_members (billing_account_id, user_id, role)
  VALUES (v_billing_account_id, p_user_id, 'owner')
  ON CONFLICT (billing_account_id, user_id) DO NOTHING;

  RETURN v_billing_account_id;
END;
$$;

REVOKE ALL ON FUNCTION ensure_personal_billing_account_for_user(BIGINT) FROM PUBLIC;

-- 2. 账户钱包生命周期函数。
CREATE FUNCTION provision_billing_account_wallet(p_billing_account_id bigint) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  v_shard_key VARCHAR(128);
BEGIN
  IF p_billing_account_id <= 0 OR NOT EXISTS (SELECT 1 FROM billing_accounts WHERE internal_id = p_billing_account_id) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid billing account wallet owner';
  END IF;

  v_shard_key := 'billing-account:' || p_billing_account_id::TEXT;
  INSERT INTO credit_wallets (billing_account_id) VALUES (p_billing_account_id) ON CONFLICT DO NOTHING;
  INSERT INTO credit_accounts (billing_account_id, account_role)
  VALUES (p_billing_account_id, 'user_available'), (p_billing_account_id, 'user_reserved')
  ON CONFLICT DO NOTHING;
  INSERT INTO credit_accounts (account_role, shard_key, metadata)
  VALUES
    ('system_issuance', v_shard_key, jsonb_build_object('scope', 'billing_account', 'billing_account_id', p_billing_account_id)),
    ('system_revenue', v_shard_key, jsonb_build_object('scope', 'billing_account', 'billing_account_id', p_billing_account_id)),
    ('system_sink', v_shard_key, jsonb_build_object('scope', 'billing_account', 'billing_account_id', p_billing_account_id))
  ON CONFLICT DO NOTHING;
END;
$$;

CREATE FUNCTION set_billing_account_wallet_status(p_billing_account_id bigint, p_status text) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  p_status := btrim(p_status);
  IF p_billing_account_id <= 0 OR p_status NOT IN ('active', 'frozen') THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid billing account wallet status command';
  END IF;
  UPDATE credit_wallets SET status = p_status, updated_at = now() WHERE billing_account_id = p_billing_account_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
END;
$$;

CREATE FUNCTION freeze_sole_owned_billing_wallets(p_user_id bigint, p_frozen boolean) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
DECLARE
  v_status TEXT := CASE WHEN p_frozen THEN 'frozen' ELSE 'active' END;
BEGIN
  IF p_user_id <= 0 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid billing wallet freeze command';
  END IF;
  UPDATE credit_wallets wallets
  SET status = v_status, updated_at = now()
  FROM billing_accounts accounts
  WHERE wallets.billing_account_id = accounts.internal_id
    AND accounts.organization_id IS NULL
    AND EXISTS (
      SELECT 1 FROM billing_account_members managers
      WHERE managers.billing_account_id = accounts.internal_id
        AND managers.user_id = p_user_id AND managers.role = 'owner'
    )
    AND NOT EXISTS (
      SELECT 1 FROM billing_account_members others
      WHERE others.billing_account_id = accounts.internal_id
        AND others.user_id <> p_user_id
    );
END;
$$;

CREATE FUNCTION remove_shared_billing_memberships_for_user(p_user_id bigint) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  IF p_user_id <= 0 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid billing membership cleanup command';
  END IF;
  DELETE FROM billing_account_members departed
  WHERE departed.user_id = p_user_id
    AND EXISTS (
      SELECT 1 FROM billing_account_members others
      WHERE others.billing_account_id = departed.billing_account_id
        AND others.user_id <> p_user_id
    );
END;
$$;

REVOKE ALL ON FUNCTION provision_billing_account_wallet(BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION set_billing_account_wallet_status(BIGINT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION freeze_sole_owned_billing_wallets(BIGINT, BOOLEAN) FROM PUBLIC;
REVOKE ALL ON FUNCTION remove_shared_billing_memberships_for_user(BIGINT) FROM PUBLIC;

-- 3. 新增归属列(全 nullable,回填后再收敛)。
ALTER TABLE credit_wallets ADD COLUMN billing_account_id BIGINT;
ALTER TABLE credit_wallet_restrictions ADD COLUMN billing_account_id BIGINT;
ALTER TABLE credit_accounts ADD COLUMN billing_account_id BIGINT;
ALTER TABLE credit_transactions ADD COLUMN billing_account_id BIGINT;
ALTER TABLE credit_holds ADD COLUMN billing_account_id BIGINT;
ALTER TABLE credit_charges ADD COLUMN billing_account_id BIGINT;
ALTER TABLE credit_reconciliation_findings ADD COLUMN billing_account_id BIGINT;
ALTER TABLE payment_dispute_debit_requests ADD COLUMN billing_account_id BIGINT;
ALTER TABLE task_billing_snapshots ADD COLUMN billing_account_id BIGINT;
ALTER TABLE ai_search_operations ADD COLUMN billing_account_id BIGINT;

-- 4. 存量归属回填:个人钱包 -> 默认个人账户;组织/共享账户钱包随后新建(0 起)。
DO $$
DECLARE
  v_user_id BIGINT;
BEGIN
  FOR v_user_id IN SELECT user_id FROM credit_wallets LOOP
    UPDATE credit_wallets
    SET billing_account_id = ensure_personal_billing_account_for_user(v_user_id)
    WHERE user_id = v_user_id AND billing_account_id IS NULL;
  END LOOP;
END;
$$;

-- 5. 下线失效引用(PK 切换前必须先解除)。
ALTER TABLE credit_accounts DROP CONSTRAINT fk_credit_accounts_wallet_user_id;
ALTER TABLE credit_transactions DROP CONSTRAINT fk_credit_transactions_wallet_user_id;
ALTER TABLE credit_holds DROP CONSTRAINT fk_credit_holds_wallet_user_id;
ALTER TABLE credit_charges DROP CONSTRAINT fk_credit_charges_wallet_user_id;
ALTER TABLE credit_wallet_restrictions DROP CONSTRAINT fk_credit_wallet_restrictions_wallet_user_id;
ALTER TABLE credit_reconciliation_findings DROP CONSTRAINT credit_reconciliation_findings_wallet_user_id_fkey;
ALTER TABLE payment_dispute_debit_requests DROP CONSTRAINT payment_dispute_debit_requests_wallet_user_id_fkey;
ALTER TABLE credit_wallets DROP CONSTRAINT credit_wallets_pkey;
ALTER TABLE credit_accounts DROP CONSTRAINT ck_credit_accounts_owner;

-- 6. 主键切换 + 遗留列放宽(均为 DROP,非破坏数据)。
ALTER TABLE credit_wallets ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE credit_wallet_restrictions ALTER COLUMN wallet_user_id DROP NOT NULL;
ALTER TABLE credit_accounts ALTER COLUMN wallet_user_id DROP NOT NULL;
ALTER TABLE credit_transactions ALTER COLUMN wallet_user_id DROP NOT NULL;
ALTER TABLE credit_holds ALTER COLUMN wallet_user_id DROP NOT NULL;
ALTER TABLE credit_charges ALTER COLUMN wallet_user_id DROP NOT NULL;
ALTER TABLE payment_dispute_debit_requests ALTER COLUMN wallet_user_id DROP NOT NULL;
ALTER TABLE credit_wallets ADD CONSTRAINT credit_wallets_pkey PRIMARY KEY (billing_account_id);
CREATE INDEX idx_credit_wallets_user_id ON credit_wallets (user_id) WHERE user_id IS NOT NULL;

-- 7. 其余回填(同一事务内确定性归属)。
-- 回填只写新增归属列,不碰金额/postings;三类不可变 guard 在回填窗口内
-- 暂停(ALTER ... DISABLE/ENABLE TRIGGER,门禁不拦截),回填后立即恢复。
ALTER TABLE credit_transactions DISABLE TRIGGER prevent_credit_transactions_mutation;
ALTER TABLE credit_accounts DISABLE TRIGGER guard_credit_account_projection;
ALTER TABLE task_billing_snapshots DISABLE TRIGGER task_billing_snapshots_immutable;
-- 7a. 账户投影归属(用户角色行随钱包走;系统分片行保持 NULL,新分片键隔离)。
-- 必须先于 7b 建钱包:NOT EXISTS 才能正确跳过已有归属行的账户,否则新旧
-- 行在同一 (billing_account_id, role) 上重复,违反 §9 唯一约束。
UPDATE credit_accounts accounts
SET billing_account_id = wallets.billing_account_id
FROM credit_wallets wallets
WHERE accounts.wallet_user_id = wallets.user_id
  AND accounts.account_role IN ('user_available', 'user_reserved')
  AND accounts.billing_account_id IS NULL;

-- 7b. 所有 Billing Account 建钱包(含 915 存量组织/空间账户,从 0 起)。
DO $$
DECLARE
  v_account_id BIGINT;
BEGIN
  FOR v_account_id IN SELECT internal_id FROM billing_accounts LOOP
    IF NOT EXISTS (
      SELECT 1 FROM credit_accounts
      WHERE billing_account_id = v_account_id AND account_role IN ('user_available', 'user_reserved')
    ) THEN
      PERFORM provision_billing_account_wallet(v_account_id);
    END IF;
  END LOOP;
END;
$$;

-- 7b2. 存量归属账户补系统分片行:回填只建了用户角色行(§7a),新分片键的
-- issuance/revenue/sink 行在此补齐(guarded,无重复)。缺少它们时
-- capture/purchase/refund/dispute 会报 account set incomplete。
INSERT INTO credit_accounts (account_role, shard_key, metadata)
SELECT roles.role, 'billing-account:' || accounts.internal_id,
       jsonb_build_object('scope', 'billing_account', 'billing_account_id', accounts.internal_id)
FROM billing_accounts accounts
CROSS JOIN (VALUES ('system_issuance'), ('system_revenue'), ('system_sink')) AS roles(role)
WHERE NOT EXISTS (
  SELECT 1 FROM credit_accounts existing
  WHERE existing.account_role = roles.role
    AND existing.shard_key = 'billing-account:' || accounts.internal_id
);

-- 7c. Restriction / Ledger / Hold / Charge 归属。
UPDATE credit_wallet_restrictions restrictions
SET billing_account_id = wallets.billing_account_id
FROM credit_wallets wallets
WHERE restrictions.wallet_user_id = wallets.user_id AND restrictions.billing_account_id IS NULL;

UPDATE credit_transactions transactions
SET billing_account_id = wallets.billing_account_id
FROM credit_wallets wallets
WHERE transactions.wallet_user_id = wallets.user_id AND transactions.billing_account_id IS NULL;

UPDATE credit_holds holds
SET billing_account_id = wallets.billing_account_id
FROM credit_wallets wallets
WHERE holds.wallet_user_id = wallets.user_id AND holds.billing_account_id IS NULL;

UPDATE credit_charges charges
SET billing_account_id = wallets.billing_account_id
FROM credit_wallets wallets
WHERE charges.wallet_user_id = wallets.user_id AND charges.billing_account_id IS NULL;

UPDATE credit_reconciliation_findings findings
SET billing_account_id = ensure_personal_billing_account_for_user(findings.wallet_user_id)
WHERE findings.wallet_user_id IS NOT NULL AND findings.billing_account_id IS NULL;

UPDATE payment_dispute_debit_requests requests
SET billing_account_id = ensure_personal_billing_account_for_user(requests.wallet_user_id)
WHERE requests.wallet_user_id IS NOT NULL AND requests.billing_account_id IS NULL;

-- 7d. Task snapshot 跟钱走:付费快照归属 Hold 所在账户;free/孤儿归属任务账户或提交人个人账户。
UPDATE task_billing_snapshots snapshots
SET billing_account_id = transactions.billing_account_id
FROM credit_transactions transactions
WHERE snapshots.hold_transaction_id = transactions.transaction_id
  AND snapshots.total_credits > 0 AND snapshots.billing_account_id IS NULL;

UPDATE task_billing_snapshots snapshots
SET billing_account_id = COALESCE(
  (SELECT tasks.billing_account_id FROM tasks WHERE tasks.public_id = snapshots.task_id),
  ensure_personal_billing_account_for_user(snapshots.user_id))
WHERE snapshots.billing_account_id IS NULL;

-- 7e. Vision 操作归属操作人个人账户(操作人保留在 user_id)。
UPDATE ai_search_operations operations
SET billing_account_id = ensure_personal_billing_account_for_user(operations.user_id)
WHERE operations.billing_account_id IS NULL;

ALTER TABLE credit_transactions ENABLE TRIGGER prevent_credit_transactions_mutation;
ALTER TABLE credit_accounts ENABLE TRIGGER guard_credit_account_projection;
ALTER TABLE task_billing_snapshots ENABLE TRIGGER task_billing_snapshots_immutable;

-- 8. 新约束收敛(ADD CONSTRAINT,门禁合规)。
ALTER TABLE credit_accounts ADD CONSTRAINT ck_credit_accounts_owner CHECK (
  (account_role IN ('user_available', 'user_reserved') AND shard_key = ''
    AND (wallet_user_id IS NOT NULL OR billing_account_id IS NOT NULL))
  OR
  (billing_account_id IS NULL AND wallet_user_id IS NULL AND account_role IN ('system_issuance', 'system_revenue', 'system_sink') AND btrim(shard_key) <> '')
);
ALTER TABLE credit_wallet_restrictions ADD CONSTRAINT ck_credit_wallet_restrictions_billing_account CHECK (billing_account_id IS NOT NULL);
ALTER TABLE credit_wallet_restrictions ADD CONSTRAINT fk_credit_wallet_restrictions_billing_account_id FOREIGN KEY (billing_account_id) REFERENCES credit_wallets(billing_account_id) ON DELETE RESTRICT;
ALTER TABLE credit_wallet_restrictions ADD CONSTRAINT fk_credit_wallet_restrictions_wallet_user_id FOREIGN KEY (wallet_user_id) REFERENCES users(id) ON DELETE RESTRICT;
ALTER TABLE credit_transactions ADD CONSTRAINT ck_credit_transactions_billing_account CHECK (billing_account_id IS NOT NULL);
ALTER TABLE credit_holds ADD CONSTRAINT ck_credit_holds_billing_account CHECK (billing_account_id IS NOT NULL);
ALTER TABLE credit_charges ADD CONSTRAINT ck_credit_charges_billing_account CHECK (billing_account_id IS NOT NULL);
ALTER TABLE payment_dispute_debit_requests ADD CONSTRAINT ck_payment_dispute_debit_request_billing_account CHECK (billing_account_id IS NOT NULL);
ALTER TABLE task_billing_snapshots ADD CONSTRAINT task_billing_snapshots_billing_account_check CHECK ((billing_account_id IS NOT NULL AND billing_account_id > 0));
ALTER TABLE ai_search_operations ADD CONSTRAINT ck_ai_search_operations_billing_account CHECK (billing_account_id IS NOT NULL);
ALTER TABLE credit_accounts ADD CONSTRAINT fk_credit_accounts_billing_account_id FOREIGN KEY (billing_account_id) REFERENCES credit_wallets(billing_account_id) ON DELETE RESTRICT;
ALTER TABLE credit_reconciliation_findings ADD CONSTRAINT fk_credit_reconciliation_findings_wallet_user_id FOREIGN KEY (wallet_user_id) REFERENCES users(id) ON UPDATE RESTRICT ON DELETE RESTRICT;
ALTER TABLE payment_dispute_debit_requests ADD CONSTRAINT fk_payment_dispute_debit_requests_wallet_user_id FOREIGN KEY (wallet_user_id) REFERENCES users(id) ON DELETE RESTRICT;

-- 9. 索引切换(新唯一/查找键;旧 user-keyed 索引下线)。
DROP INDEX uq_credit_wallet_restrictions_active_source;
CREATE UNIQUE INDEX uq_credit_wallet_restrictions_active_source
  ON credit_wallet_restrictions (billing_account_id, restriction_type, source, related_object_type, related_object_id)
  WHERE released_at IS NULL;
DROP INDEX idx_credit_wallet_restrictions_wallet_active;
CREATE INDEX idx_credit_wallet_restrictions_wallet_active
  ON credit_wallet_restrictions (billing_account_id, created_at DESC, restriction_id)
  WHERE released_at IS NULL;
CREATE UNIQUE INDEX uq_credit_accounts_billing_role ON credit_accounts (billing_account_id, asset_code, account_role) WHERE billing_account_id IS NOT NULL;
DROP INDEX uq_credit_accounts_system_shard_role;
CREATE UNIQUE INDEX uq_credit_accounts_system_shard_role ON credit_accounts (asset_code, account_role, shard_key) WHERE wallet_user_id IS NULL AND billing_account_id IS NULL;
DROP INDEX idx_credit_transactions_wallet_id_desc;
CREATE INDEX idx_credit_transactions_wallet_id_desc ON credit_transactions (billing_account_id, id DESC);
CREATE INDEX idx_credit_transactions_legacy_wallet_id_desc ON credit_transactions (wallet_user_id, id DESC) WHERE wallet_user_id IS NOT NULL;
DROP INDEX idx_credit_transactions_capture_user_created;
CREATE INDEX idx_credit_transactions_capture_user_created
  ON credit_transactions (billing_account_id, created_at)
  INCLUDE (command_amount)
  WHERE command_type = 'capture';
DROP INDEX idx_credit_holds_wallet_status;
CREATE INDEX idx_credit_holds_wallet_status ON credit_holds (billing_account_id, status, created_at DESC);
DROP INDEX idx_credit_reconciliation_findings_wallet;
CREATE INDEX idx_credit_reconciliation_findings_wallet
  ON credit_reconciliation_findings (billing_account_id, id DESC) WHERE billing_account_id IS NOT NULL;
DROP INDEX idx_payment_dispute_debit_requests_wallet;
CREATE INDEX idx_payment_dispute_debit_requests_wallet
  ON payment_dispute_debit_requests (billing_account_id, created_at DESC, id DESC);
CREATE INDEX idx_task_billing_snapshots_account_created ON task_billing_snapshots USING btree (billing_account_id, created_at);

-- 10. 账户契约函数切换(新建 v3/v2 + 下线 user-keyed 版本)。
CREATE FUNCTION execute_credit_purchase_v2(
  p_billing_account_id BIGINT,
  p_amount BIGINT,
  p_idempotency_key TEXT,
  p_actor_type TEXT,
  p_actor_id TEXT,
  p_source TEXT,
  p_reason TEXT,
  p_business_reference_type TEXT,
  p_business_reference_id TEXT
)
RETURNS TABLE (
  transaction_id VARCHAR(64), command_type VARCHAR(24), command_amount BIGINT,
  billing_account_id BIGINT, available_balance_after BIGINT, reserved_balance_after BIGINT,
  replayed BOOLEAN, created_at TIMESTAMPTZ(3)
)
AS $$
DECLARE
  v_payload JSONB;
  v_hash TEXT;
  v_existing_hash TEXT;
  v_transaction_id VARCHAR(64);
  v_available_id BIGINT;
  v_reserved_id BIGINT;
  v_issuance_id BIGINT;
  v_available_balance BIGINT;
  v_reserved_balance BIGINT;
  v_shard_key VARCHAR(128);
BEGIN
  p_idempotency_key := btrim(p_idempotency_key);
  p_actor_type := btrim(p_actor_type);
  p_actor_id := btrim(p_actor_id);
  p_source := btrim(p_source);
  p_reason := btrim(p_reason);
  p_business_reference_type := btrim(p_business_reference_type);
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_billing_account_id <= 0 OR p_amount <= 0 OR p_amount > 9000000000000000
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160
     OR p_actor_type <> 'service' OR p_actor_id <> 'payments'
     OR p_source <> 'stripe_purchase' OR p_reason = ''
     OR p_business_reference_type <> 'payment_order'
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit purchase command';
  END IF;
  v_payload := jsonb_build_object(
    'command_type', 'purchase_credit', 'billing_account_id', p_billing_account_id, 'amount', p_amount,
    'related_transaction_id', '',
    'actor', jsonb_build_object('type', p_actor_type, 'id', p_actor_id),
    'source', p_source, 'reason', p_reason,
    'business_reference', jsonb_build_object('type', p_business_reference_type, 'id', p_business_reference_id)
  );
  v_hash := encode(sha256(convert_to(v_payload::TEXT, 'UTF8')), 'hex');
  PERFORM pg_advisory_xact_lock(hashtextextended(p_idempotency_key, 5283921922134398276));
  SELECT existing.request_hash INTO v_existing_hash
  FROM credit_transactions existing
  WHERE existing.idempotency_key = p_idempotency_key;
  IF FOUND THEN
    IF v_existing_hash <> v_hash THEN
      RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict';
    END IF;
    RETURN QUERY
    SELECT existing.transaction_id, existing.command_type, existing.command_amount,
      existing.billing_account_id, existing.available_balance_after, existing.reserved_balance_after,
      TRUE, existing.created_at
    FROM credit_transactions existing
    WHERE existing.idempotency_key = p_idempotency_key;
    RETURN;
  END IF;
  PERFORM 1 FROM credit_wallets wallets WHERE wallets.billing_account_id = p_billing_account_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
  v_shard_key := 'billing-account:' || p_billing_account_id::TEXT;
  PERFORM accounts.id
  FROM credit_accounts accounts
  WHERE (accounts.billing_account_id = p_billing_account_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.billing_account_id IS NULL AND accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_issuance')
  ORDER BY accounts.id FOR UPDATE;
  SELECT
    MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_available'),
    MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_reserved'),
    MAX(accounts.id) FILTER (WHERE accounts.account_role = 'system_issuance'),
    MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available'),
    MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved')
  INTO v_available_id, v_reserved_id, v_issuance_id, v_available_balance, v_reserved_balance
  FROM credit_accounts accounts
  WHERE (accounts.billing_account_id = p_billing_account_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.billing_account_id IS NULL AND accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_issuance');
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_issuance_id IS NULL THEN
    RAISE EXCEPTION 'credit wallet account set is incomplete';
  END IF;
  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(
    clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8'
  )), 'hex'), 1, 32);
  INSERT INTO credit_transactions (
    transaction_id, billing_account_id, command_type, idempotency_key, request_hash,
    actor_type, actor_id, source, reason, business_reference_type, business_reference_id,
    command_amount, posting_count, available_balance_after, reserved_balance_after
  ) VALUES (
    v_transaction_id, p_billing_account_id, 'purchase_credit', p_idempotency_key, v_hash,
    p_actor_type, p_actor_id, p_source, p_reason, p_business_reference_type, p_business_reference_id,
    p_amount, 2, v_available_balance + p_amount, v_reserved_balance
  );
  INSERT INTO credit_postings (transaction_id, account_id, amount)
  VALUES (v_transaction_id, v_issuance_id, -p_amount), (v_transaction_id, v_available_id, p_amount);
  RETURN QUERY
  SELECT created.transaction_id, created.command_type, created.command_amount,
    created.billing_account_id, created.available_balance_after, created.reserved_balance_after,
    FALSE, created.created_at
  FROM credit_transactions created
  WHERE created.transaction_id = v_transaction_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_purchase_v2(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema,
    trusted_schema
  );
END;
$$;

REVOKE ALL ON FUNCTION execute_credit_purchase_v2(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;


CREATE FUNCTION execute_credit_purchase_refund_hold_v2(
  p_billing_account_id BIGINT,
  p_amount BIGINT,
  p_idempotency_key TEXT,
  p_actor_type TEXT,
  p_actor_id TEXT,
  p_source TEXT,
  p_reason TEXT,
  p_business_reference_type TEXT,
  p_business_reference_id TEXT
)
RETURNS TABLE (
  transaction_id VARCHAR(64), command_type VARCHAR(24), command_amount BIGINT,
  billing_account_id BIGINT, available_balance_after BIGINT, reserved_balance_after BIGINT,
  replayed BOOLEAN, created_at TIMESTAMPTZ(3)
)
AS $$
DECLARE
  v_payload JSONB;
  v_hash TEXT;
  v_existing_hash TEXT;
  v_transaction_id VARCHAR(64);
  v_wallet_status VARCHAR(20);
  v_available_id BIGINT;
  v_reserved_id BIGINT;
  v_available_balance BIGINT;
  v_reserved_balance BIGINT;
BEGIN
  p_idempotency_key := btrim(p_idempotency_key);
  p_actor_type := btrim(p_actor_type);
  p_actor_id := btrim(p_actor_id);
  p_source := btrim(p_source);
  p_reason := btrim(p_reason);
  p_business_reference_type := btrim(p_business_reference_type);
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_billing_account_id <= 0 OR p_amount <= 0 OR p_amount > 9000000000000000
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160
     OR p_actor_type <> 'service' OR p_actor_id <> 'payments'
     OR p_source <> 'stripe_refund_hold' OR p_reason = ''
     OR p_business_reference_type <> 'payment_refund'
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit purchase refund hold command';
  END IF;

  v_payload := jsonb_build_object(
    'command_type', 'hold', 'billing_account_id', p_billing_account_id, 'amount', p_amount,
    'related_transaction_id', '',
    'actor', jsonb_build_object('type', p_actor_type, 'id', p_actor_id),
    'source', p_source, 'reason', p_reason,
    'business_reference', jsonb_build_object('type', p_business_reference_type, 'id', p_business_reference_id)
  );
  v_hash := encode(sha256(convert_to(v_payload::TEXT, 'UTF8')), 'hex');
  PERFORM pg_advisory_xact_lock(hashtextextended(p_idempotency_key, 5283921922134398276));
  SELECT request_hash INTO v_existing_hash
  FROM credit_transactions
  WHERE idempotency_key = p_idempotency_key;
  IF FOUND THEN
    IF v_existing_hash <> v_hash THEN
      RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict';
    END IF;
    RETURN QUERY SELECT existing.transaction_id, existing.command_type, existing.command_amount,
      existing.billing_account_id, existing.available_balance_after, existing.reserved_balance_after,
      TRUE, existing.created_at
    FROM credit_transactions existing
    WHERE existing.idempotency_key = p_idempotency_key;
    RETURN;
  END IF;

  SELECT wallets.status INTO v_wallet_status
  FROM credit_wallets wallets
  WHERE wallets.billing_account_id = p_billing_account_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
  IF v_wallet_status <> 'active' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB003', MESSAGE = 'credit wallet frozen';
  END IF;

  PERFORM accounts.id
  FROM credit_accounts accounts
  WHERE accounts.billing_account_id = p_billing_account_id
    AND accounts.account_role IN ('user_available', 'user_reserved')
  ORDER BY accounts.id
  FOR UPDATE;
  SELECT MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_reserved'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved')
  INTO v_available_id, v_reserved_id, v_available_balance, v_reserved_balance
  FROM credit_accounts accounts
  WHERE accounts.billing_account_id = p_billing_account_id
    AND accounts.account_role IN ('user_available', 'user_reserved');
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_available_balance < p_amount THEN
    RAISE EXCEPTION USING ERRCODE = 'IB004', MESSAGE = 'insufficient credits';
  END IF;

  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(
    clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8'
  )), 'hex'), 1, 32);
  INSERT INTO credit_transactions (
    transaction_id, billing_account_id, command_type, idempotency_key, request_hash,
    actor_type, actor_id, source, reason, business_reference_type, business_reference_id,
    command_amount, posting_count, available_balance_after, reserved_balance_after
  ) VALUES (
    v_transaction_id, p_billing_account_id, 'hold', p_idempotency_key, v_hash,
    p_actor_type, p_actor_id, p_source, p_reason, p_business_reference_type, p_business_reference_id,
    p_amount, 2, v_available_balance - p_amount, v_reserved_balance + p_amount
  );
  INSERT INTO credit_postings (transaction_id, account_id, amount)
  VALUES (v_transaction_id, v_available_id, -p_amount), (v_transaction_id, v_reserved_id, p_amount);
  INSERT INTO credit_holds (hold_transaction_id, billing_account_id, amount)
  VALUES (v_transaction_id, p_billing_account_id, p_amount);

  RETURN QUERY SELECT created.transaction_id, created.command_type, created.command_amount,
    created.billing_account_id, created.available_balance_after, created.reserved_balance_after,
    FALSE, created.created_at
  FROM credit_transactions created
  WHERE created.transaction_id = v_transaction_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_purchase_refund_hold_v2(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
END;
$$;

REVOKE ALL ON FUNCTION execute_credit_purchase_refund_hold_v2(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;


CREATE FUNCTION execute_credit_purchase_refund_v2(
  p_billing_account_id BIGINT, p_hold_transaction_id TEXT, p_idempotency_key TEXT,
  p_actor_type TEXT, p_actor_id TEXT, p_source TEXT, p_reason TEXT,
  p_business_reference_type TEXT, p_business_reference_id TEXT
)
RETURNS TABLE (
  transaction_id VARCHAR(64), command_type VARCHAR(24), command_amount BIGINT,
  billing_account_id BIGINT, available_balance_after BIGINT, reserved_balance_after BIGINT,
  replayed BOOLEAN, created_at TIMESTAMPTZ(3)
)
AS $$
DECLARE
  v_payload JSONB; v_hash TEXT; v_existing_hash TEXT; v_transaction_id VARCHAR(64);
  v_amount BIGINT; v_hold_status VARCHAR(20); v_available_id BIGINT; v_reserved_id BIGINT;
  v_issuance_id BIGINT; v_available_balance BIGINT; v_reserved_balance BIGINT; v_shard_key VARCHAR(128);
BEGIN
  p_hold_transaction_id := btrim(p_hold_transaction_id); p_idempotency_key := btrim(p_idempotency_key);
  p_actor_type := btrim(p_actor_type); p_actor_id := btrim(p_actor_id); p_source := btrim(p_source);
  p_reason := btrim(p_reason); p_business_reference_type := btrim(p_business_reference_type);
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_billing_account_id <= 0 OR p_hold_transaction_id = '' OR length(p_hold_transaction_id) > 64
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160
     OR p_actor_type <> 'service' OR p_actor_id <> 'payments' OR p_source <> 'stripe_refund'
     OR p_reason = '' OR p_business_reference_type <> 'payment_refund'
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit purchase refund command';
  END IF;
  v_payload := jsonb_build_object(
    'command_type', 'purchase_refund', 'billing_account_id', p_billing_account_id,
    'related_transaction_id', p_hold_transaction_id,
    'actor', jsonb_build_object('type', p_actor_type, 'id', p_actor_id),
    'source', p_source, 'reason', p_reason,
    'business_reference', jsonb_build_object('type', p_business_reference_type, 'id', p_business_reference_id)
  );
  v_hash := encode(sha256(convert_to(v_payload::TEXT, 'UTF8')), 'hex');
  PERFORM pg_advisory_xact_lock(hashtextextended(p_idempotency_key, 5283921922134398276));
  SELECT request_hash INTO v_existing_hash FROM credit_transactions WHERE idempotency_key = p_idempotency_key;
  IF FOUND THEN
    IF v_existing_hash <> v_hash THEN RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict'; END IF;
    RETURN QUERY SELECT existing.transaction_id, existing.command_type, existing.command_amount,
      existing.billing_account_id, existing.available_balance_after, existing.reserved_balance_after, TRUE, existing.created_at
    FROM credit_transactions existing WHERE existing.idempotency_key = p_idempotency_key;
    RETURN;
  END IF;
  SELECT holds.amount, holds.status INTO v_amount, v_hold_status FROM credit_holds holds
  WHERE holds.hold_transaction_id = p_hold_transaction_id AND holds.billing_account_id = p_billing_account_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'IB005', MESSAGE = 'credit hold not found'; END IF;
  IF v_hold_status <> 'held' THEN RAISE EXCEPTION USING ERRCODE = 'IB006', MESSAGE = 'credit hold already settled'; END IF;
  PERFORM 1 FROM credit_wallets wallets WHERE wallets.billing_account_id = p_billing_account_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found'; END IF;
  v_shard_key := 'billing-account:' || p_billing_account_id::TEXT;
  PERFORM accounts.id FROM credit_accounts accounts
  WHERE (accounts.billing_account_id = p_billing_account_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.billing_account_id IS NULL AND accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_issuance')
  ORDER BY accounts.id FOR UPDATE;
  SELECT MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_reserved'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'system_issuance'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved')
  INTO v_available_id, v_reserved_id, v_issuance_id, v_available_balance, v_reserved_balance
  FROM credit_accounts accounts
  WHERE (accounts.billing_account_id = p_billing_account_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.billing_account_id IS NULL AND accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_issuance');
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_issuance_id IS NULL OR v_reserved_balance < v_amount THEN
    RAISE EXCEPTION USING ERRCODE = 'IB004', MESSAGE = 'insufficient credits';
  END IF;
  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(
    clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8'
  )), 'hex'), 1, 32);
  INSERT INTO credit_transactions (
    transaction_id, billing_account_id, command_type, idempotency_key, request_hash,
    actor_type, actor_id, source, reason, business_reference_type, business_reference_id,
    related_transaction_id, command_amount, posting_count, available_balance_after, reserved_balance_after
  ) VALUES (
    v_transaction_id, p_billing_account_id, 'purchase_refund', p_idempotency_key, v_hash,
    p_actor_type, p_actor_id, p_source, p_reason, p_business_reference_type, p_business_reference_id,
    p_hold_transaction_id, v_amount, 2, v_available_balance, v_reserved_balance - v_amount
  );
  INSERT INTO credit_postings (transaction_id, account_id, amount)
  VALUES (v_transaction_id, v_reserved_id, -v_amount), (v_transaction_id, v_issuance_id, v_amount);
  UPDATE credit_holds
  SET status = 'captured',
      captured_amount = captured_amount + v_amount,
      capture_transaction_id = v_transaction_id,
      settled_at = now()
  WHERE hold_transaction_id = p_hold_transaction_id;
  RETURN QUERY SELECT created.transaction_id, created.command_type, created.command_amount,
    created.billing_account_id, created.available_balance_after, created.reserved_balance_after, FALSE, created.created_at
  FROM credit_transactions created WHERE created.transaction_id = v_transaction_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_purchase_refund_v2(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
END;
$$;

REVOKE ALL ON FUNCTION execute_credit_purchase_refund_v2(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;


CREATE FUNCTION execute_credit_payment_dispute_debit_v2(
  p_billing_account_id BIGINT,
  p_requested_amount BIGINT,
  p_idempotency_key TEXT,
  p_actor_type TEXT,
  p_actor_id TEXT,
  p_source TEXT,
  p_reason TEXT,
  p_business_reference_type TEXT,
  p_business_reference_id TEXT
)
RETURNS TABLE (
  transaction_id VARCHAR(64), command_type VARCHAR(24), command_amount BIGINT,
  billing_account_id BIGINT, available_balance_after BIGINT, reserved_balance_after BIGINT,
  replayed BOOLEAN, created_at TIMESTAMPTZ(3), requested_amount BIGINT, unrecovered_amount BIGINT
)
AS $$
DECLARE
  v_payload JSONB;
  v_hash TEXT;
  v_existing_hash TEXT;
  v_transaction_id VARCHAR(64);
  v_debited_amount BIGINT;
  v_available_id BIGINT;
  v_reserved_id BIGINT;
  v_sink_id BIGINT;
  v_available_balance BIGINT;
  v_reserved_balance BIGINT;
  v_shard_key VARCHAR(128);
  v_created_at TIMESTAMPTZ(3);
BEGIN
  p_idempotency_key := btrim(p_idempotency_key);
  p_actor_type := btrim(p_actor_type);
  p_actor_id := btrim(p_actor_id);
  p_source := btrim(p_source);
  p_reason := btrim(p_reason);
  p_business_reference_type := btrim(p_business_reference_type);
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_billing_account_id <= 0 OR p_requested_amount <= 0 OR p_requested_amount > 9000000000000000
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160
     OR p_actor_type <> 'service' OR p_actor_id <> 'payments'
     OR p_source <> 'stripe_dispute' OR p_reason = ''
     OR p_business_reference_type <> 'payment_dispute'
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit payment dispute debit command';
  END IF;

  v_payload := jsonb_build_object(
    'command_type', 'payment_dispute_debit', 'billing_account_id', p_billing_account_id, 'requested_amount', p_requested_amount,
    'actor', jsonb_build_object('type', p_actor_type, 'id', p_actor_id),
    'source', p_source, 'reason', p_reason,
    'business_reference', jsonb_build_object('type', p_business_reference_type, 'id', p_business_reference_id)
  );
  v_hash := encode(sha256(convert_to(v_payload::TEXT, 'UTF8')), 'hex');
  PERFORM pg_advisory_xact_lock(hashtextextended(p_idempotency_key, 5283921922134398276));

  SELECT request_hash INTO v_existing_hash
  FROM payment_dispute_debit_requests
  WHERE idempotency_key = p_idempotency_key;
  IF FOUND THEN
    IF v_existing_hash <> v_hash THEN
      RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict';
    END IF;
    RETURN QUERY
    SELECT COALESCE(request.credit_transaction_id, ''), 'payment_dispute_debit'::VARCHAR(24), request.debited_amount,
      request.billing_account_id, request.available_balance_after, request.reserved_balance_after, TRUE, request.created_at,
      request.requested_amount, request.requested_amount - request.debited_amount
    FROM payment_dispute_debit_requests request
    WHERE request.idempotency_key = p_idempotency_key;
    RETURN;
  END IF;
  IF EXISTS (SELECT 1 FROM credit_transactions WHERE idempotency_key = p_idempotency_key) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict';
  END IF;

  PERFORM 1 FROM credit_wallets wallets WHERE wallets.billing_account_id = p_billing_account_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
  v_shard_key := 'billing-account:' || p_billing_account_id::TEXT;
  PERFORM accounts.id FROM credit_accounts accounts
  WHERE (accounts.billing_account_id = p_billing_account_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.billing_account_id IS NULL AND accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_sink')
  ORDER BY accounts.id FOR UPDATE;
  SELECT MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_reserved'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'system_sink'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved')
  INTO v_available_id, v_reserved_id, v_sink_id, v_available_balance, v_reserved_balance
  FROM credit_accounts accounts
  WHERE (accounts.billing_account_id = p_billing_account_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.billing_account_id IS NULL AND accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_sink');
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_sink_id IS NULL THEN
    RAISE EXCEPTION 'credit wallet account set is incomplete';
  END IF;
  v_debited_amount := LEAST(p_requested_amount, v_available_balance);

  IF v_debited_amount = 0 THEN
    INSERT INTO payment_dispute_debit_requests (
      id, billing_account_id, wallet_user_id, idempotency_key, request_hash, requested_amount, debited_amount,
      available_balance_after, reserved_balance_after,
      business_reference_type, business_reference_id
    ) VALUES (
      gen_random_uuid(), p_billing_account_id, NULL, p_idempotency_key, v_hash, p_requested_amount, 0,
      v_available_balance, v_reserved_balance,
      p_business_reference_type, p_business_reference_id
    ) RETURNING payment_dispute_debit_requests.created_at INTO v_created_at;
    RETURN QUERY SELECT ''::VARCHAR(64), 'payment_dispute_debit'::VARCHAR(24), 0::BIGINT,
      p_billing_account_id, v_available_balance, v_reserved_balance, FALSE, v_created_at, p_requested_amount, p_requested_amount;
    RETURN;
  END IF;

  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(
    clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8'
  )), 'hex'), 1, 32);
  INSERT INTO credit_transactions (
    transaction_id, billing_account_id, command_type, idempotency_key, request_hash,
    actor_type, actor_id, source, reason, business_reference_type, business_reference_id,
    command_amount, posting_count, available_balance_after, reserved_balance_after
  ) VALUES (
    v_transaction_id, p_billing_account_id, 'payment_dispute_debit', p_idempotency_key, v_hash,
    p_actor_type, p_actor_id, p_source, p_reason, p_business_reference_type, p_business_reference_id,
    v_debited_amount, 2, v_available_balance - v_debited_amount, v_reserved_balance
  );
  INSERT INTO credit_postings (transaction_id, account_id, amount)
  VALUES (v_transaction_id, v_available_id, -v_debited_amount), (v_transaction_id, v_sink_id, v_debited_amount);
  INSERT INTO payment_dispute_debit_requests (
      id, billing_account_id, wallet_user_id, idempotency_key, request_hash, requested_amount, debited_amount,
      available_balance_after, reserved_balance_after,
      credit_transaction_id, business_reference_type, business_reference_id
    ) VALUES (
      gen_random_uuid(), p_billing_account_id, NULL, p_idempotency_key, v_hash, p_requested_amount, v_debited_amount,
      v_available_balance - v_debited_amount, v_reserved_balance,
      v_transaction_id, p_business_reference_type, p_business_reference_id
  ) RETURNING payment_dispute_debit_requests.created_at INTO v_created_at;
  RETURN QUERY SELECT v_transaction_id, 'payment_dispute_debit'::VARCHAR(24), v_debited_amount,
    p_billing_account_id, v_available_balance - v_debited_amount, v_reserved_balance, FALSE, v_created_at,
    p_requested_amount, p_requested_amount - v_debited_amount;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

CREATE FUNCTION execute_credit_payment_dispute_restore_v2(
  p_billing_account_id BIGINT,
  p_original_transaction_id TEXT,
  p_amount BIGINT,
  p_idempotency_key TEXT,
  p_actor_type TEXT,
  p_actor_id TEXT,
  p_source TEXT,
  p_reason TEXT,
  p_business_reference_type TEXT,
  p_business_reference_id TEXT
)
RETURNS TABLE (
  transaction_id VARCHAR(64), command_type VARCHAR(24), command_amount BIGINT,
  billing_account_id BIGINT, available_balance_after BIGINT, reserved_balance_after BIGINT,
  replayed BOOLEAN, created_at TIMESTAMPTZ(3)
)
AS $$
DECLARE
  v_payload JSONB;
  v_hash TEXT;
  v_existing_hash TEXT;
  v_transaction_id VARCHAR(64);
  v_original_amount BIGINT;
  v_restored_amount BIGINT;
  v_available_id BIGINT;
  v_reserved_id BIGINT;
  v_sink_id BIGINT;
  v_available_balance BIGINT;
  v_reserved_balance BIGINT;
  v_shard_key VARCHAR(128);
BEGIN
  p_original_transaction_id := btrim(p_original_transaction_id);
  p_idempotency_key := btrim(p_idempotency_key);
  p_actor_type := btrim(p_actor_type);
  p_actor_id := btrim(p_actor_id);
  p_source := btrim(p_source);
  p_reason := btrim(p_reason);
  p_business_reference_type := btrim(p_business_reference_type);
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_billing_account_id <= 0 OR p_original_transaction_id = '' OR length(p_original_transaction_id) > 64
     OR p_amount <= 0 OR p_amount > 9000000000000000
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160
     OR p_actor_type <> 'service' OR p_actor_id <> 'payments'
     OR p_source <> 'stripe_dispute' OR p_reason = ''
     OR p_business_reference_type <> 'payment_dispute'
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit payment dispute restore command';
  END IF;

  v_payload := jsonb_build_object(
    'command_type', 'payment_dispute_restore', 'billing_account_id', p_billing_account_id, 'amount', p_amount,
    'related_transaction_id', p_original_transaction_id,
    'actor', jsonb_build_object('type', p_actor_type, 'id', p_actor_id),
    'source', p_source, 'reason', p_reason,
    'business_reference', jsonb_build_object('type', p_business_reference_type, 'id', p_business_reference_id)
  );
  v_hash := encode(sha256(convert_to(v_payload::TEXT, 'UTF8')), 'hex');
  PERFORM pg_advisory_xact_lock(hashtextextended(p_idempotency_key, 5283921922134398276));
  SELECT request_hash INTO v_existing_hash FROM credit_transactions WHERE idempotency_key = p_idempotency_key;
  IF FOUND THEN
    IF v_existing_hash <> v_hash THEN
      RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict';
    END IF;
    RETURN QUERY SELECT existing.transaction_id, existing.command_type, existing.command_amount,
      existing.billing_account_id, existing.available_balance_after, existing.reserved_balance_after,
      TRUE, existing.created_at
    FROM credit_transactions existing WHERE existing.idempotency_key = p_idempotency_key;
    RETURN;
  END IF;
  IF EXISTS (SELECT 1 FROM payment_dispute_debit_requests WHERE idempotency_key = p_idempotency_key) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict';
  END IF;

  SELECT original.command_amount INTO v_original_amount
  FROM credit_transactions original
  WHERE original.transaction_id = p_original_transaction_id AND original.billing_account_id = p_billing_account_id
    AND original.command_type = 'payment_dispute_debit' AND original.actor_type = 'service' AND original.actor_id = 'payments'
    AND original.source = 'stripe_dispute' AND original.business_reference_type = p_business_reference_type
    AND original.business_reference_id = p_business_reference_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid payment dispute debit reference';
  END IF;
  SELECT COALESCE(SUM(restores.command_amount), 0) INTO v_restored_amount
  FROM credit_transactions restores
  WHERE restores.related_transaction_id = p_original_transaction_id AND restores.command_type = 'payment_dispute_restore';
  IF p_amount > v_original_amount - v_restored_amount THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'payment dispute restore exceeds debited amount';
  END IF;

  PERFORM 1 FROM credit_wallets wallets WHERE wallets.billing_account_id = p_billing_account_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;
  v_shard_key := 'billing-account:' || p_billing_account_id::TEXT;
  PERFORM accounts.id FROM credit_accounts accounts
  WHERE (accounts.billing_account_id = p_billing_account_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.billing_account_id IS NULL AND accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_sink')
  ORDER BY accounts.id FOR UPDATE;
  SELECT MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'user_reserved'),
         MAX(accounts.id) FILTER (WHERE accounts.account_role = 'system_sink'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_available'),
         MAX(accounts.balance) FILTER (WHERE accounts.account_role = 'user_reserved')
  INTO v_available_id, v_reserved_id, v_sink_id, v_available_balance, v_reserved_balance
  FROM credit_accounts accounts
  WHERE (accounts.billing_account_id = p_billing_account_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.billing_account_id IS NULL AND accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role = 'system_sink');
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_sink_id IS NULL THEN
    RAISE EXCEPTION 'credit wallet account set is incomplete';
  END IF;
  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(
    clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8'
  )), 'hex'), 1, 32);
  INSERT INTO credit_transactions (
    transaction_id, billing_account_id, command_type, idempotency_key, request_hash,
    actor_type, actor_id, source, reason, business_reference_type, business_reference_id,
    related_transaction_id, command_amount, posting_count, available_balance_after, reserved_balance_after
  ) VALUES (
    v_transaction_id, p_billing_account_id, 'payment_dispute_restore', p_idempotency_key, v_hash,
    p_actor_type, p_actor_id, p_source, p_reason, p_business_reference_type, p_business_reference_id,
    p_original_transaction_id, p_amount, 2, v_available_balance + p_amount, v_reserved_balance
  );
  INSERT INTO credit_postings (transaction_id, account_id, amount)
  VALUES (v_transaction_id, v_sink_id, -p_amount), (v_transaction_id, v_available_id, p_amount);
  RETURN QUERY SELECT created.transaction_id, created.command_type, created.command_amount,
    created.billing_account_id, created.available_balance_after, created.reserved_balance_after,
    FALSE, created.created_at
  FROM credit_transactions created WHERE created.transaction_id = v_transaction_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

-- statement:begin
DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_payment_dispute_debit_v2(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
  EXECUTE format(
    'ALTER FUNCTION %I.execute_credit_payment_dispute_restore_v2(BIGINT, TEXT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp',
    trusted_schema, trusted_schema
  );
END;
$$;
-- statement:end

REVOKE ALL ON FUNCTION execute_credit_payment_dispute_debit_v2(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION execute_credit_payment_dispute_restore_v2(BIGINT, TEXT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;


CREATE FUNCTION provision_billing_account_wallet_on_account_insert() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  PERFORM provision_billing_account_wallet(NEW.internal_id);
  RETURN NEW;
END;
$$;

CREATE TRIGGER provision_billing_account_wallet_after_account_insert
AFTER INSERT ON billing_accounts
FOR EACH ROW
EXECUTE FUNCTION provision_billing_account_wallet_on_account_insert();

REVOKE ALL ON FUNCTION provision_billing_account_wallet_on_account_insert() FROM PUBLIC;

DROP FUNCTION execute_credit_command_v2(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);

DROP FUNCTION provision_credit_wallet(BIGINT);

DROP FUNCTION set_credit_wallet_status(BIGINT, TEXT);

DROP FUNCTION execute_credit_purchase_v1(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);

DROP FUNCTION execute_credit_purchase_refund_hold_v1(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);

DROP FUNCTION execute_credit_purchase_refund_v1(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);

DROP FUNCTION execute_credit_payment_dispute_debit_v1(BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);

DROP FUNCTION execute_credit_payment_dispute_restore_v1(BIGINT, TEXT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);

-- 11. 存量函数体账户化替换(与 fragments 快照一致)。
DROP FUNCTION IF EXISTS create_credit_wallet_restriction(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT);
CREATE FUNCTION create_credit_wallet_restriction(
  p_billing_account_id BIGINT,
  p_restriction_type TEXT,
  p_source TEXT,
  p_related_object_type TEXT,
  p_related_object_id TEXT,
  p_reason TEXT,
  p_actor_type TEXT,
  p_actor_id TEXT
)
RETURNS TABLE (restriction_id TEXT, replayed BOOLEAN)
AS $$
DECLARE
  v_restriction_id UUID;
BEGIN
  p_restriction_type := btrim(p_restriction_type);
  p_source := btrim(p_source);
  p_related_object_type := btrim(p_related_object_type);
  p_related_object_id := btrim(p_related_object_id);
  p_reason := btrim(p_reason);
  p_actor_type := btrim(p_actor_type);
  p_actor_id := btrim(p_actor_id);
  IF p_actor_type = 'admin' THEN
    p_reason := '';
  END IF;

  IF p_billing_account_id <= 0
     OR p_restriction_type = '' OR length(p_restriction_type) > 64
     OR p_source = '' OR length(p_source) > 64
     OR p_related_object_type = '' OR length(p_related_object_type) > 64
     OR p_related_object_id = '' OR length(p_related_object_id) > 160
     OR (p_actor_type <> 'admin' AND p_reason = '') OR length(p_reason) > 1000
     OR p_actor_type NOT IN ('admin', 'system', 'service')
     OR p_actor_id = '' OR length(p_actor_id) > 128 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit wallet restriction';
  END IF;

  PERFORM 1 FROM credit_wallets wallets WHERE wallets.billing_account_id = p_billing_account_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;

  INSERT INTO credit_wallet_restrictions (
    billing_account_id,
    restriction_type,
    source,
    related_object_type,
    related_object_id,
    reason,
    created_by_actor_type,
    created_by_actor_id
  )
  VALUES (
    p_billing_account_id,
    p_restriction_type,
    p_source,
    p_related_object_type,
    p_related_object_id,
    p_reason,
    p_actor_type,
    p_actor_id
  )
  ON CONFLICT (
    billing_account_id,
    restriction_type,
    source,
    related_object_type,
    related_object_id
  ) WHERE released_at IS NULL
  DO NOTHING
  RETURNING credit_wallet_restrictions.restriction_id INTO v_restriction_id;

  IF FOUND THEN
    RETURN QUERY SELECT v_restriction_id::TEXT, FALSE;
    RETURN;
  END IF;
  SELECT restrictions.restriction_id INTO v_restriction_id
  FROM credit_wallet_restrictions restrictions
  WHERE restrictions.billing_account_id = p_billing_account_id
    AND restrictions.restriction_type = p_restriction_type
    AND restrictions.source = p_source
    AND restrictions.related_object_type = p_related_object_type
    AND restrictions.related_object_id = p_related_object_id
    AND restrictions.released_at IS NULL;

  RETURN QUERY SELECT v_restriction_id::TEXT, TRUE;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

CREATE OR REPLACE FUNCTION release_credit_wallet_restriction(
  p_restriction_id TEXT,
  p_actor_type TEXT,
  p_actor_id TEXT,
  p_reason TEXT
)
RETURNS TABLE (restriction_id TEXT, replayed BOOLEAN)
AS $$
DECLARE
  v_restriction_id UUID;
  v_wallet_billing_account_id BIGINT;
  v_replayed BOOLEAN;
BEGIN
  p_restriction_id := btrim(p_restriction_id);
  p_actor_type := btrim(p_actor_type);
  p_actor_id := btrim(p_actor_id);
  IF p_actor_type = 'admin' THEN
    p_reason := '';
  END IF;
  p_reason := btrim(p_reason);

  IF p_restriction_id !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
     OR p_actor_type NOT IN ('admin', 'system', 'service')
     OR p_actor_id = '' OR length(p_actor_id) > 128
     OR (p_actor_type <> 'admin' AND p_reason = '') OR length(p_reason) > 1000 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit wallet restriction release';
  END IF;

  v_restriction_id := p_restriction_id::UUID;
  SELECT restrictions.billing_account_id INTO v_wallet_billing_account_id
  FROM credit_wallet_restrictions restrictions
  WHERE restrictions.restriction_id = v_restriction_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB012', MESSAGE = 'credit wallet restriction not found';
  END IF;

  PERFORM 1 FROM credit_wallets wallets WHERE wallets.billing_account_id = v_wallet_billing_account_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found';
  END IF;

  UPDATE credit_wallet_restrictions restrictions
  SET released_at = clock_timestamp(),
      released_by_actor_type = p_actor_type,
      released_by_actor_id = p_actor_id,
      release_reason = p_reason
  WHERE restrictions.restriction_id = v_restriction_id
    AND restrictions.released_at IS NULL;

  v_replayed := NOT FOUND;
  RETURN QUERY SELECT v_restriction_id::TEXT, v_replayed;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

CREATE FUNCTION execute_credit_command_v3(
  p_command_type TEXT, p_billing_account_id BIGINT, p_amount BIGINT, p_related_transaction_id TEXT,
  p_idempotency_key TEXT, p_actor_type TEXT, p_actor_id TEXT, p_source TEXT, p_reason TEXT,
  p_business_reference_type TEXT, p_business_reference_id TEXT
)
RETURNS TABLE (
  transaction_id VARCHAR(64), command_type VARCHAR(24), command_amount BIGINT,
  billing_account_id BIGINT, available_balance_after BIGINT, reserved_balance_after BIGINT,
  replayed BOOLEAN, created_at TIMESTAMPTZ(3)
)
AS $$
DECLARE
  v_payload JSONB; v_hash TEXT; v_existing_hash TEXT; v_transaction_id VARCHAR(64);
  v_amount BIGINT := p_amount; v_wallet_status VARCHAR(20); v_hold_status VARCHAR(20);
  v_hold_amount BIGINT; v_captured_amount BIGINT; v_released_amount BIGINT;
  v_capture_transaction_id VARCHAR(64); v_release_transaction_id VARCHAR(64);
  v_charge_amount BIGINT; v_refunded_amount BIGINT; v_original_command_type VARCHAR(24); v_original_amount BIGINT;
  v_original_actor_type VARCHAR(24); v_original_source VARCHAR(64); v_original_posting_count INT; v_corrected_amount BIGINT;
  v_original_from_id BIGINT; v_original_to_id BIGINT; v_original_positive_amount BIGINT;
  v_original_negative_amount BIGINT; v_original_posting_rows BIGINT;
  v_original_related_transaction_id VARCHAR(64);
  v_available_id BIGINT; v_reserved_id BIGINT;
  v_issuance_id BIGINT; v_revenue_id BIGINT; v_sink_id BIGINT; v_available_balance BIGINT;
  v_reserved_balance BIGINT; v_available_after BIGINT; v_reserved_after BIGINT;
  v_from_id BIGINT; v_to_id BIGINT; v_terminal_status VARCHAR(20); v_shard_key VARCHAR(128);
BEGIN
  p_command_type := btrim(p_command_type); p_related_transaction_id := btrim(COALESCE(p_related_transaction_id, ''));
  p_idempotency_key := btrim(p_idempotency_key); p_actor_type := btrim(p_actor_type); p_actor_id := btrim(p_actor_id);
  p_source := btrim(p_source); p_reason := btrim(p_reason); p_business_reference_type := btrim(p_business_reference_type);
  IF p_actor_type = 'admin' THEN
    p_reason := '';
  END IF;
  p_business_reference_id := btrim(p_business_reference_id);
  IF p_billing_account_id <= 0 OR p_command_type NOT IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction')
     OR p_idempotency_key = '' OR length(p_idempotency_key) > 160 OR p_actor_type NOT IN ('admin', 'system', 'service', 'user')
     OR p_actor_id = '' OR length(p_actor_id) > 128 OR p_source = '' OR length(p_source) > 64 OR (p_actor_type <> 'admin' AND p_reason = '')
     OR p_business_reference_type = '' OR length(p_business_reference_type) > 64
     OR p_business_reference_id = '' OR length(p_business_reference_id) > 160
     OR (p_command_type IN ('deduction', 'correction') AND p_actor_type <> 'admin')
     OR (p_command_type IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction') AND (p_amount <= 0 OR p_amount > 9000000000000000))
     OR (p_command_type IN ('capture', 'release') AND p_related_transaction_id = '')
     OR (p_command_type IN ('refund', 'correction') AND (p_related_transaction_id = '' OR length(p_related_transaction_id) > 64)) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit command';
  END IF;
  v_payload := jsonb_build_object('command_type', p_command_type, 'billing_account_id', p_billing_account_id, 'amount', p_amount,
    'related_transaction_id', p_related_transaction_id, 'actor', jsonb_build_object('type', p_actor_type, 'id', p_actor_id),
    'source', p_source, 'reason', p_reason, 'business_reference', jsonb_build_object('type', p_business_reference_type, 'id', p_business_reference_id));
  v_hash := encode(sha256(convert_to(v_payload::TEXT, 'UTF8')), 'hex');
  PERFORM pg_advisory_xact_lock(hashtextextended(p_idempotency_key, 5283921922134398276));
  SELECT request_hash INTO v_existing_hash FROM credit_transactions WHERE idempotency_key = p_idempotency_key;
  IF FOUND THEN
    IF v_existing_hash <> v_hash THEN RAISE EXCEPTION USING ERRCODE = 'IB001', MESSAGE = 'credit idempotency conflict'; END IF;
    RETURN QUERY SELECT existing.transaction_id, existing.command_type, existing.command_amount, existing.billing_account_id,
      existing.available_balance_after, existing.reserved_balance_after, TRUE, existing.created_at
      FROM credit_transactions existing WHERE existing.idempotency_key = p_idempotency_key;
    RETURN;
  END IF;
  v_shard_key := 'billing-account:' || p_billing_account_id::TEXT;
  IF p_command_type = 'grant' THEN
    INSERT INTO credit_wallets (billing_account_id) VALUES (p_billing_account_id) ON CONFLICT DO NOTHING;
    INSERT INTO credit_accounts (billing_account_id, account_role) VALUES (p_billing_account_id, 'user_available'), (p_billing_account_id, 'user_reserved') ON CONFLICT DO NOTHING;
    INSERT INTO credit_accounts (account_role, shard_key, metadata) VALUES
      ('system_issuance', v_shard_key, jsonb_build_object('scope', 'billing_account', 'billing_account_id', p_billing_account_id)),
      ('system_revenue', v_shard_key, jsonb_build_object('scope', 'billing_account', 'billing_account_id', p_billing_account_id)),
      ('system_sink', v_shard_key, jsonb_build_object('scope', 'billing_account', 'billing_account_id', p_billing_account_id)) ON CONFLICT DO NOTHING;
  END IF;
  IF p_command_type IN ('capture', 'release') THEN
    SELECT amount, status, captured_amount, released_amount, capture_transaction_id, release_transaction_id
      INTO v_hold_amount, v_hold_status, v_captured_amount, v_released_amount, v_capture_transaction_id, v_release_transaction_id
      FROM credit_holds holds
      WHERE holds.hold_transaction_id = p_related_transaction_id AND holds.billing_account_id = p_billing_account_id FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'IB005', MESSAGE = 'credit hold not found'; END IF;
    IF v_hold_status <> 'held' THEN RAISE EXCEPTION USING ERRCODE = 'IB006', MESSAGE = 'credit hold already settled'; END IF;
    IF (p_command_type = 'capture' AND v_capture_transaction_id IS NOT NULL)
       OR (p_command_type = 'release' AND v_release_transaction_id IS NOT NULL) THEN
      RAISE EXCEPTION USING ERRCODE = 'IB006', MESSAGE = 'credit hold settlement command already recorded';
    END IF;
    IF v_amount > v_hold_amount - v_captured_amount - v_released_amount THEN
      RAISE EXCEPTION USING ERRCODE = 'IB013', MESSAGE = 'credit hold settlement exceeds remaining amount';
    END IF;
  ELSIF p_command_type = 'refund' THEN
    SELECT amount, refunded_amount INTO v_charge_amount, v_refunded_amount FROM credit_charges charges
      WHERE charges.capture_transaction_id = p_related_transaction_id AND charges.billing_account_id = p_billing_account_id FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'IB007', MESSAGE = 'credit charge not found'; END IF;
    IF p_amount > v_charge_amount - v_refunded_amount THEN RAISE EXCEPTION USING ERRCODE = 'IB008', MESSAGE = 'credit refund exceeds remaining charge'; END IF;
  ELSIF p_command_type = 'correction' THEN
    SELECT original_transaction.command_type, original_transaction.command_amount, original_transaction.actor_type, original_transaction.source, original_transaction.posting_count, COALESCE(original_transaction.related_transaction_id, '') INTO v_original_command_type, v_original_amount, v_original_actor_type, v_original_source, v_original_posting_count, v_original_related_transaction_id
    FROM credit_transactions original_transaction
    WHERE original_transaction.transaction_id = p_related_transaction_id AND original_transaction.billing_account_id = p_billing_account_id
    FOR UPDATE;
    IF NOT FOUND OR v_original_command_type NOT IN ('grant', 'deduction', 'refund') OR (v_original_actor_type <> 'admin' AND NOT (v_original_command_type = 'grant' AND v_original_source = 'reward_redemption')) THEN
      RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit correction';
    END IF;
    SELECT COUNT(*), MAX(amount) FILTER (WHERE amount > 0), MIN(amount) FILTER (WHERE amount < 0),
      MAX(account_id) FILTER (WHERE amount < 0), MAX(account_id) FILTER (WHERE amount > 0)
    INTO v_original_posting_rows, v_original_positive_amount, v_original_negative_amount, v_original_from_id, v_original_to_id
    FROM credit_postings original_posting WHERE original_posting.transaction_id = p_related_transaction_id;
    IF v_original_posting_count <> 2 OR v_original_posting_rows <> 2 OR v_original_positive_amount <> v_original_amount OR v_original_negative_amount <> -v_original_amount OR v_original_from_id IS NULL OR v_original_to_id IS NULL THEN
      RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid credit correction postings';
    END IF;
    SELECT COALESCE(SUM(correction_transaction.command_amount), 0) INTO v_corrected_amount
    FROM credit_transactions correction_transaction
    WHERE correction_transaction.related_transaction_id = p_related_transaction_id AND correction_transaction.command_type = 'correction';
    IF p_amount > v_original_amount - v_corrected_amount THEN
      RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'credit correction exceeds original amount';
    END IF;
    IF v_original_command_type = 'refund' THEN
      SELECT refunded_amount INTO v_refunded_amount FROM credit_charges charge
      WHERE charge.capture_transaction_id = v_original_related_transaction_id AND charge.billing_account_id = p_billing_account_id FOR UPDATE;
      IF NOT FOUND OR p_amount > v_refunded_amount THEN
        RAISE EXCEPTION USING ERRCODE = 'IB009', MESSAGE = 'invalid refund correction';
      END IF;
    END IF;
  END IF;
  SELECT status INTO v_wallet_status FROM credit_wallets wallets WHERE wallets.billing_account_id = p_billing_account_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE = 'IB002', MESSAGE = 'credit wallet not found'; END IF;
  IF v_wallet_status <> 'active'
     AND p_command_type NOT IN ('capture', 'release')
     AND NOT (p_command_type IN ('refund', 'correction') AND p_actor_type = 'admin') THEN
    RAISE EXCEPTION USING ERRCODE = 'IB003', MESSAGE = 'credit wallet frozen';
  END IF;
  IF p_command_type IN ('grant', 'deduction', 'hold')
     AND EXISTS (
       SELECT 1 FROM credit_wallet_restrictions restrictions
       WHERE restrictions.billing_account_id = p_billing_account_id AND restrictions.released_at IS NULL
     ) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB011', MESSAGE = 'credit wallet restricted';
  END IF;
  PERFORM id FROM credit_accounts accounts WHERE (accounts.billing_account_id = p_billing_account_id AND accounts.account_role IN ('user_available', 'user_reserved'))
     OR (accounts.billing_account_id IS NULL AND accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role IN ('system_issuance', 'system_revenue', 'system_sink')) ORDER BY accounts.id FOR UPDATE;
  SELECT MAX(id) FILTER (WHERE account_role = 'user_available'), MAX(id) FILTER (WHERE account_role = 'user_reserved'),
    MAX(id) FILTER (WHERE account_role = 'system_issuance'), MAX(id) FILTER (WHERE account_role = 'system_revenue'),
    MAX(id) FILTER (WHERE account_role = 'system_sink'), MAX(balance) FILTER (WHERE account_role = 'user_available'),
    MAX(balance) FILTER (WHERE account_role = 'user_reserved') INTO v_available_id, v_reserved_id, v_issuance_id, v_revenue_id,
    v_sink_id, v_available_balance, v_reserved_balance FROM credit_accounts accounts WHERE
    (accounts.billing_account_id = p_billing_account_id AND accounts.account_role IN ('user_available', 'user_reserved')) OR
    (accounts.billing_account_id IS NULL AND accounts.wallet_user_id IS NULL AND accounts.shard_key = v_shard_key AND accounts.account_role IN ('system_issuance', 'system_revenue', 'system_sink'));
  IF v_available_id IS NULL OR v_reserved_id IS NULL OR v_issuance_id IS NULL OR v_revenue_id IS NULL OR v_sink_id IS NULL THEN
    RAISE EXCEPTION 'credit wallet account set is incomplete';
  END IF;
  CASE p_command_type
    WHEN 'grant' THEN v_from_id := v_issuance_id; v_to_id := v_available_id;
    WHEN 'deduction' THEN v_from_id := v_available_id; v_to_id := v_sink_id;
    WHEN 'correction' THEN v_from_id := v_original_to_id; v_to_id := v_original_from_id;
    WHEN 'hold' THEN v_from_id := v_available_id; v_to_id := v_reserved_id;
    WHEN 'capture' THEN v_from_id := v_reserved_id; v_to_id := v_revenue_id;
    WHEN 'release' THEN v_from_id := v_reserved_id; v_to_id := v_available_id;
    WHEN 'refund' THEN v_from_id := v_revenue_id; v_to_id := v_available_id;
  END CASE;
  v_available_after := v_available_balance; v_reserved_after := v_reserved_balance;
  IF v_from_id = v_available_id THEN v_available_after := v_available_after - v_amount; END IF;
  IF v_to_id = v_available_id THEN v_available_after := v_available_after + v_amount; END IF;
  IF v_from_id = v_reserved_id THEN v_reserved_after := v_reserved_after - v_amount; END IF;
  IF v_to_id = v_reserved_id THEN v_reserved_after := v_reserved_after + v_amount; END IF;
  IF v_available_after < 0 OR v_reserved_after < 0 THEN RAISE EXCEPTION USING ERRCODE = 'IB004', MESSAGE = 'insufficient credits'; END IF;
  v_transaction_id := 'ctx_' || substr(encode(sha256(convert_to(clock_timestamp()::TEXT || random()::TEXT || p_idempotency_key, 'UTF8')), 'hex'), 1, 32);
  INSERT INTO credit_transactions (transaction_id, billing_account_id, command_type, idempotency_key, request_hash, actor_type, actor_id,
    source, reason, business_reference_type, business_reference_id, related_transaction_id, command_amount, posting_count, available_balance_after, reserved_balance_after)
    VALUES (v_transaction_id, p_billing_account_id, p_command_type, p_idempotency_key, v_hash, p_actor_type, p_actor_id, p_source, p_reason,
    p_business_reference_type, p_business_reference_id, NULLIF(p_related_transaction_id, ''), v_amount, 2, v_available_after, v_reserved_after);
  INSERT INTO credit_postings (transaction_id, account_id, amount) VALUES (v_transaction_id, v_from_id, -v_amount), (v_transaction_id, v_to_id, v_amount);
  IF p_command_type = 'hold' THEN
    INSERT INTO credit_holds (hold_transaction_id, billing_account_id, amount) VALUES (v_transaction_id, p_billing_account_id, v_amount);
  ELSIF p_command_type IN ('capture', 'release') THEN
    IF p_command_type = 'capture' THEN
      v_captured_amount := v_captured_amount + v_amount;
      v_capture_transaction_id := v_transaction_id;
    ELSE
      v_released_amount := v_released_amount + v_amount;
      v_release_transaction_id := v_transaction_id;
    END IF;
    IF v_captured_amount + v_released_amount = v_hold_amount THEN
      v_terminal_status := CASE WHEN v_captured_amount > 0 THEN 'captured' ELSE 'released' END;
    ELSE
      v_terminal_status := 'held';
    END IF;
    UPDATE credit_holds
    SET captured_amount = v_captured_amount,
        released_amount = v_released_amount,
        status = v_terminal_status,
        capture_transaction_id = v_capture_transaction_id,
        release_transaction_id = v_release_transaction_id,
        settled_at = CASE WHEN v_terminal_status = 'held' THEN NULL ELSE now() END
    WHERE hold_transaction_id = p_related_transaction_id;
    IF p_command_type = 'capture' THEN INSERT INTO credit_charges (capture_transaction_id, billing_account_id, amount) VALUES (v_transaction_id, p_billing_account_id, v_amount); END IF;
  ELSIF p_command_type = 'refund' THEN
    UPDATE credit_charges SET refunded_amount = refunded_amount + v_amount, updated_at = now() WHERE capture_transaction_id = p_related_transaction_id;
  ELSIF p_command_type = 'correction' AND v_original_command_type = 'refund' THEN
    UPDATE credit_charges SET refunded_amount = refunded_amount - v_amount, updated_at = now() WHERE capture_transaction_id = v_original_related_transaction_id;
  END IF;
  RETURN QUERY SELECT created.transaction_id, created.command_type, created.command_amount, created.billing_account_id, created.available_balance_after,
    created.reserved_balance_after, FALSE, created.created_at FROM credit_transactions created WHERE created.transaction_id = v_transaction_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

DO $$
DECLARE trusted_schema TEXT := current_schema();
BEGIN
  EXECUTE format('ALTER FUNCTION %I.create_credit_wallet_restriction(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
  EXECUTE format('ALTER FUNCTION %I.release_credit_wallet_restriction(TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
  EXECUTE format('ALTER FUNCTION %I.execute_credit_command_v3(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) SET search_path TO %I, pg_catalog, pg_temp', trusted_schema, trusted_schema);
END;
$$;

REVOKE ALL ON FUNCTION create_credit_wallet_restriction(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION release_credit_wallet_restriction(TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION execute_credit_command_v3(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT) FROM PUBLIC;


CREATE OR REPLACE FUNCTION execute_credit_reconciliation(p_run_id BIGINT)
RETURNS VOID AS $$
DECLARE
  v_stale_after INTERVAL;
  v_finding RECORD;
  v_restriction_id TEXT;
BEGIN
  IF NOT pg_try_advisory_xact_lock(hashtextextended(current_schema(), 5283921922134398278)) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB302', MESSAGE = 'credit reconciliation already running';
  END IF;
  UPDATE credit_reconciliation_runs SET status = 'running', started_at = clock_timestamp(), completed_at = NULL,
    error_message = '', attempt_count = attempt_count + 1
  WHERE id = p_run_id AND status IN ('queued', 'failed')
  RETURNING stale_hold_after INTO v_stale_after;
  IF NOT FOUND THEN RETURN; END IF;

  CREATE TEMP TABLE reconciliation_scan (
    finding_type TEXT, severity TEXT, risk TEXT, billing_account_id BIGINT,
    subject_type TEXT, subject_id TEXT, details JSONB
  ) ON COMMIT DROP;

  INSERT INTO reconciliation_scan
  SELECT 'ledger_postings_invalid', 'critical', 'high', transactions.billing_account_id,
    'transaction', transactions.transaction_id,
    jsonb_build_object('declared_posting_count', transactions.posting_count, 'actual_posting_count', COUNT(postings.id),
      'asset_count', COUNT(DISTINCT postings.asset_code), 'posting_sum', COALESCE(SUM(postings.amount), 0),
      'debit_amount', COALESCE(-SUM(postings.amount) FILTER (WHERE postings.amount < 0), 0),
      'credit_amount', COALESCE(SUM(postings.amount) FILTER (WHERE postings.amount > 0), 0),
      'command_amount', transactions.command_amount)
  FROM credit_transactions transactions
  LEFT JOIN credit_postings postings ON postings.transaction_id = transactions.transaction_id
  GROUP BY transactions.transaction_id, transactions.billing_account_id, transactions.posting_count, transactions.command_amount
  HAVING COUNT(postings.id) < 2 OR COUNT(postings.id) <> transactions.posting_count
      OR COUNT(DISTINCT postings.asset_code) <> 1
      OR BOOL_OR(postings.asset_code IS DISTINCT FROM transactions.asset_code)
      OR COALESCE(SUM(postings.amount), 0) <> 0
      OR COALESCE(-SUM(postings.amount) FILTER (WHERE postings.amount < 0), 0) <> transactions.command_amount
      OR COALESCE(SUM(postings.amount) FILTER (WHERE postings.amount > 0), 0) <> transactions.command_amount;

  INSERT INTO reconciliation_scan
  SELECT 'command_shape_invalid', 'critical', 'high', transactions.billing_account_id, 'transaction', transactions.transaction_id,
    jsonb_build_object('command_type', transactions.command_type, 'actor_type', transactions.actor_type,
      'related_transaction_id', transactions.related_transaction_id, 'command_amount', transactions.command_amount)
  FROM credit_transactions transactions
  WHERE transactions.command_type NOT IN ('grant', 'deduction', 'hold', 'capture', 'release', 'refund', 'correction', 'purchase_credit', 'purchase_refund', 'payment_dispute_debit', 'payment_dispute_restore')
    OR (transactions.command_type IN ('deduction', 'correction') AND transactions.actor_type <> 'admin')
    OR (transactions.command_type = 'purchase_credit' AND (
      transactions.actor_type <> 'service' OR transactions.actor_id <> 'payments'
      OR transactions.source <> 'stripe_purchase'
      OR transactions.business_reference_type <> 'payment_order'
    ))
    OR (transactions.command_type = 'purchase_refund' AND (
      transactions.actor_type <> 'service' OR transactions.actor_id <> 'payments'
      OR transactions.source <> 'stripe_refund'
      OR transactions.business_reference_type <> 'payment_refund'
    ))
    OR (transactions.command_type = 'payment_dispute_debit' AND (
      transactions.actor_type <> 'service' OR transactions.actor_id <> 'payments'
      OR transactions.source <> 'stripe_dispute'
      OR transactions.business_reference_type <> 'payment_dispute'
    ))
    OR (transactions.command_type = 'payment_dispute_restore' AND (
      transactions.actor_type <> 'service' OR transactions.actor_id <> 'payments'
      OR transactions.source <> 'stripe_dispute'
      OR transactions.business_reference_type <> 'payment_dispute'
    ))
    OR transactions.command_amount <= 0;

  INSERT INTO reconciliation_scan
  SELECT 'wallet_projection_mismatch', 'critical', 'high', accounts.billing_account_id,
    'account', accounts.id::TEXT,
    jsonb_build_object('account_role', accounts.account_role, 'projected_balance', accounts.balance,
      'ledger_balance', COALESCE(SUM(postings.amount), 0))
  FROM credit_accounts accounts
  LEFT JOIN credit_postings postings ON postings.account_id = accounts.id
  WHERE accounts.billing_account_id IS NOT NULL
  GROUP BY accounts.id, accounts.billing_account_id, accounts.account_role, accounts.balance
  HAVING accounts.balance <> COALESCE(SUM(postings.amount), 0);

  INSERT INTO reconciliation_scan
  SELECT 'stale_hold', 'warning', 'low', holds.billing_account_id, 'hold', holds.hold_transaction_id,
    jsonb_build_object('created_at', holds.created_at, 'age_seconds', EXTRACT(EPOCH FROM clock_timestamp() - holds.created_at)::BIGINT)
  FROM credit_holds holds WHERE holds.status = 'held' AND holds.created_at < clock_timestamp() - v_stale_after;

  INSERT INTO reconciliation_scan
  SELECT 'task_hold_missing_billing_snapshot', 'critical', 'high', transactions.billing_account_id,
    'hold', transactions.transaction_id,
    jsonb_build_object('task_id', transactions.business_reference_id, 'billing_snapshot_exists', FALSE)
  FROM credit_transactions transactions
  LEFT JOIN task_billing_snapshots snapshots ON snapshots.hold_transaction_id = transactions.transaction_id
  WHERE transactions.command_type = 'hold' AND transactions.business_reference_type = 'task'
    AND snapshots.task_id IS NULL;

  INSERT INTO reconciliation_scan
  SELECT 'task_trace_missing', 'warning', 'low', snapshots.billing_account_id,
    'task', snapshots.task_id,
    jsonb_build_object('hold_transaction_id', snapshots.hold_transaction_id, 'settlement_state', settlements.state)
  FROM task_billing_snapshots snapshots
  JOIN credit_transactions transactions ON transactions.transaction_id = snapshots.hold_transaction_id
    AND transactions.command_type = 'hold' AND transactions.command_amount = snapshots.total_credits
    AND transactions.billing_account_id = snapshots.billing_account_id
    AND transactions.business_reference_type = 'task' AND transactions.business_reference_id = snapshots.task_id
  JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  JOIN credit_holds holds ON holds.hold_transaction_id = snapshots.hold_transaction_id
    AND holds.billing_account_id = snapshots.billing_account_id AND holds.amount = snapshots.total_credits
    AND holds.status = settlements.state
  LEFT JOIN tasks ON tasks.public_id = snapshots.task_id
  WHERE snapshots.total_credits > 0 AND tasks.public_id IS NULL
    AND settlements.state IN ('held', 'captured', 'released')
    AND (settlements.state = 'held' OR EXISTS (
      SELECT 1 FROM credit_transactions terminal
      WHERE terminal.transaction_id IN (settlements.capture_transaction_id, settlements.release_transaction_id)
        AND terminal.command_amount > 0
        AND terminal.billing_account_id = snapshots.billing_account_id
        AND terminal.related_transaction_id = snapshots.hold_transaction_id
        AND terminal.business_reference_type = 'task' AND terminal.business_reference_id = snapshots.task_id
    ))
    AND NOT EXISTS (
      SELECT 1
      FROM credit_transactions ledger_transactions
      LEFT JOIN LATERAL (
        SELECT COUNT(*) AS posting_count, COUNT(DISTINCT postings.asset_code) AS asset_count,
          BOOL_OR(postings.asset_code IS DISTINCT FROM ledger_transactions.asset_code) AS asset_mismatch,
          COALESCE(SUM(postings.amount), 0) AS posting_sum,
          COALESCE(-SUM(postings.amount) FILTER (WHERE postings.amount < 0), 0) AS debit_amount,
          COALESCE(SUM(postings.amount) FILTER (WHERE postings.amount > 0), 0) AS credit_amount
        FROM credit_postings postings
        WHERE postings.transaction_id = ledger_transactions.transaction_id
      ) ledger ON TRUE
      WHERE ledger_transactions.transaction_id IN (transactions.transaction_id, settlements.capture_transaction_id, settlements.release_transaction_id)
        AND (ledger.posting_count < 2 OR ledger.posting_count <> ledger_transactions.posting_count
          OR ledger.asset_count <> 1 OR ledger.asset_mismatch OR ledger.posting_sum <> 0
          OR ledger.debit_amount <> ledger_transactions.command_amount
          OR ledger.credit_amount <> ledger_transactions.command_amount)
    );

  INSERT INTO reconciliation_scan
  SELECT 'task_billing_fact_missing', 'critical', 'high', snapshots.billing_account_id, 'task', snapshots.task_id,
    jsonb_build_object('hold_transaction_id', snapshots.hold_transaction_id, 'hold_exists', holds.hold_transaction_id IS NOT NULL,
      'settlement_exists', settlements.task_id IS NOT NULL, 'hold_command', hold_transactions.command_type,
      'hold_command_amount', hold_transactions.command_amount, 'expected_amount', snapshots.total_credits,
      'hold_wallet_matches', hold_transactions.billing_account_id = snapshots.billing_account_id,
      'hold_reference_matches', hold_transactions.business_reference_type = 'task' AND hold_transactions.business_reference_id = snapshots.task_id,
      'hold_projection_matches', holds.billing_account_id = snapshots.billing_account_id AND holds.amount = snapshots.total_credits,
      'settlement_state', settlements.state, 'capture_command', capture_tx.command_type,
      'release_command', release_tx.command_type)
  FROM task_billing_snapshots snapshots
  LEFT JOIN credit_transactions hold_transactions ON hold_transactions.transaction_id = snapshots.hold_transaction_id
  LEFT JOIN credit_holds holds ON holds.hold_transaction_id = snapshots.hold_transaction_id
  LEFT JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  LEFT JOIN credit_transactions capture_tx ON capture_tx.transaction_id = settlements.capture_transaction_id
  LEFT JOIN credit_transactions release_tx ON release_tx.transaction_id = settlements.release_transaction_id
  WHERE snapshots.total_credits > 0 AND (
    holds.hold_transaction_id IS NULL OR settlements.task_id IS NULL
    OR hold_transactions.command_type IS DISTINCT FROM 'hold'
    OR hold_transactions.command_amount IS DISTINCT FROM snapshots.total_credits
    OR hold_transactions.billing_account_id IS DISTINCT FROM snapshots.billing_account_id
    OR hold_transactions.business_reference_type IS DISTINCT FROM 'task'
    OR hold_transactions.business_reference_id IS DISTINCT FROM snapshots.task_id
    OR holds.billing_account_id IS DISTINCT FROM snapshots.billing_account_id
    OR holds.amount IS DISTINCT FROM snapshots.total_credits
    OR settlements.state NOT IN ('held', 'captured', 'released')
    OR holds.status IS DISTINCT FROM settlements.state
    OR settlements.captured_credits <> settlements.actual_output_count * snapshots.unit_credits
    OR settlements.released_credits <> snapshots.total_credits - settlements.captured_credits
    OR (settlements.capture_transaction_id IS NOT NULL AND (capture_tx.command_type IS DISTINCT FROM 'capture' OR capture_tx.command_amount IS DISTINCT FROM settlements.captured_credits))
    OR (settlements.release_transaction_id IS NOT NULL AND (release_tx.command_type IS DISTINCT FROM 'release' OR release_tx.command_amount IS DISTINCT FROM settlements.released_credits))
  );

  INSERT INTO reconciliation_scan
  SELECT CASE
      WHEN tasks.status IN ('pending', 'running') THEN 'active_task_not_held'
      WHEN tasks.status IN ('completed', 'failed', 'cancelled') THEN 'terminal_task_settlement_mismatch'
    END,
    'critical', 'high', snapshots.billing_account_id, 'task', tasks.public_id,
      jsonb_build_object('task_status', tasks.status, 'settlement_state', settlements.state,
      'capture_transaction_id', settlements.capture_transaction_id, 'release_transaction_id', settlements.release_transaction_id)
  FROM task_billing_snapshots snapshots
  JOIN tasks ON tasks.public_id = snapshots.task_id
  LEFT JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  LEFT JOIN credit_holds holds ON holds.hold_transaction_id = snapshots.hold_transaction_id
  WHERE snapshots.total_credits > 0 AND (
    (tasks.status IN ('pending', 'running') AND (settlements.state IS DISTINCT FROM 'held' OR holds.status IS DISTINCT FROM 'held'))
    OR (tasks.status IN ('completed', 'failed', 'cancelled') AND (
      settlements.actual_output_count IS DISTINCT FROM (SELECT COUNT(*) FROM task_outputs outputs WHERE outputs.task_id = tasks.internal_id)
      OR settlements.state IS DISTINCT FROM CASE
        WHEN EXISTS (SELECT 1 FROM task_outputs outputs WHERE outputs.task_id = tasks.internal_id) THEN 'captured'
        ELSE 'released'
      END
      OR holds.status IS DISTINCT FROM CASE
        WHEN EXISTS (SELECT 1 FROM task_outputs outputs WHERE outputs.task_id = tasks.internal_id) THEN 'captured'
        ELSE 'released'
      END
    )));

  INSERT INTO reconciliation_scan
  SELECT 'task_terminal_transaction_count', 'critical', 'high', snapshots.billing_account_id, 'task', tasks.public_id,
    jsonb_build_object('task_status', tasks.status, 'expected_count',
      (CASE WHEN settlements.captured_credits > 0 THEN 1 ELSE 0 END) +
      (CASE WHEN settlements.released_credits > 0 THEN 1 ELSE 0 END),
      'actual_count', COUNT(terminals.transaction_id))
  FROM task_billing_snapshots snapshots
  JOIN tasks ON tasks.public_id = snapshots.task_id
  JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  LEFT JOIN credit_transactions terminals ON terminals.related_transaction_id = snapshots.hold_transaction_id
    AND terminals.command_type IN ('capture', 'release')
    AND terminals.billing_account_id = snapshots.billing_account_id
    AND terminals.business_reference_type = 'task' AND terminals.business_reference_id = tasks.public_id
  WHERE snapshots.total_credits > 0 AND tasks.status IN ('completed', 'failed', 'cancelled')
  GROUP BY snapshots.billing_account_id, tasks.public_id, tasks.status, settlements.captured_credits, settlements.released_credits
  HAVING COUNT(terminals.transaction_id) <>
    (CASE WHEN settlements.captured_credits > 0 THEN 1 ELSE 0 END) +
    (CASE WHEN settlements.released_credits > 0 THEN 1 ELSE 0 END);

  INSERT INTO reconciliation_scan
  WITH conflicts AS (
    SELECT idempotency_key, COUNT(*) AS transaction_count, COUNT(DISTINCT request_hash) AS request_hash_count
    FROM credit_transactions GROUP BY idempotency_key HAVING COUNT(DISTINCT request_hash) > 1
  )
  SELECT 'idempotency_request_hash_conflict', 'critical', 'high', transactions.billing_account_id,
    'idempotency_key', conflicts.idempotency_key,
    jsonb_build_object('transaction_count', conflicts.transaction_count, 'request_hash_count', conflicts.request_hash_count)
  FROM conflicts JOIN credit_transactions transactions USING (idempotency_key)
  GROUP BY conflicts.idempotency_key, conflicts.transaction_count, conflicts.request_hash_count, transactions.billing_account_id;

  INSERT INTO reconciliation_scan
  SELECT 'task_settlement_reference_invalid', 'critical', 'high', snapshots.billing_account_id, 'task', snapshots.task_id,
    jsonb_build_object('settlement_state', settlements.state, 'capture_transaction_id', settlements.capture_transaction_id,
      'release_transaction_id', settlements.release_transaction_id)
  FROM task_billing_snapshots snapshots
  JOIN task_billing_settlements settlements ON settlements.task_id = snapshots.task_id
  LEFT JOIN credit_transactions capture_tx ON capture_tx.transaction_id = settlements.capture_transaction_id
  LEFT JOIN credit_transactions release_tx ON release_tx.transaction_id = settlements.release_transaction_id
  WHERE settlements.state IN ('captured', 'released') AND (
    (settlements.captured_credits > 0) IS DISTINCT FROM (settlements.capture_transaction_id IS NOT NULL)
    OR (settlements.released_credits > 0) IS DISTINCT FROM (settlements.release_transaction_id IS NOT NULL)
    OR (settlements.capture_transaction_id IS NOT NULL AND (
      capture_tx.command_type IS DISTINCT FROM 'capture'
      OR capture_tx.command_amount IS DISTINCT FROM settlements.captured_credits
      OR capture_tx.billing_account_id IS DISTINCT FROM snapshots.billing_account_id
      OR capture_tx.related_transaction_id IS DISTINCT FROM snapshots.hold_transaction_id
      OR capture_tx.business_reference_type IS DISTINCT FROM 'task'
      OR capture_tx.business_reference_id IS DISTINCT FROM snapshots.task_id
    ))
    OR (settlements.release_transaction_id IS NOT NULL AND (
      release_tx.command_type IS DISTINCT FROM 'release'
      OR release_tx.command_amount IS DISTINCT FROM settlements.released_credits
      OR release_tx.billing_account_id IS DISTINCT FROM snapshots.billing_account_id
      OR release_tx.related_transaction_id IS DISTINCT FROM snapshots.hold_transaction_id
      OR release_tx.business_reference_type IS DISTINCT FROM 'task'
      OR release_tx.business_reference_id IS DISTINCT FROM snapshots.task_id
    ))
  );

  INSERT INTO reconciliation_scan
  SELECT 'duplicate_business_reference', 'error', 'high', billing_account_id, 'business_reference',
    business_reference_type || ':' || business_reference_id || ':' || command_type,
    jsonb_build_object('command_type', command_type, 'transaction_count', COUNT(*))
  FROM credit_transactions
  GROUP BY billing_account_id, business_reference_type, business_reference_id, command_type HAVING COUNT(*) > 1;

  INSERT INTO reconciliation_scan
  SELECT 'correction_amount_exceeds_original', 'critical', 'high', original.billing_account_id,
    'transaction', original.transaction_id,
    jsonb_build_object('original_command_type', original.command_type, 'original_amount', original.command_amount,
      'corrected_amount', SUM(corrections.command_amount))
  FROM credit_transactions original
  JOIN credit_transactions corrections ON corrections.command_type = 'correction'
    AND corrections.related_transaction_id = original.transaction_id
  GROUP BY original.transaction_id, original.billing_account_id, original.command_type, original.command_amount, original.actor_type
  HAVING original.command_type NOT IN ('grant', 'deduction', 'refund')
    OR original.actor_type <> 'admin' OR SUM(corrections.command_amount) > original.command_amount;

  INSERT INTO reconciliation_scan
  SELECT 'refund_amount_invalid', 'critical', 'high', charges.billing_account_id, 'charge', charges.capture_transaction_id,
    jsonb_build_object('charge_amount', charges.amount, 'projected_refunded_amount', charges.refunded_amount,
      'ledger_refunded_amount', COALESCE(refunds.amount, 0) - COALESCE(corrections.amount, 0))
  FROM credit_charges charges
  LEFT JOIN LATERAL (
    SELECT SUM(command_amount) AS amount FROM credit_transactions
    WHERE command_type = 'refund' AND related_transaction_id = charges.capture_transaction_id
  ) refunds ON TRUE
  LEFT JOIN LATERAL (
    SELECT SUM(correction.command_amount) AS amount
    FROM credit_transactions correction
    JOIN credit_transactions refund ON refund.transaction_id = correction.related_transaction_id
    WHERE correction.command_type = 'correction' AND refund.command_type = 'refund'
      AND refund.related_transaction_id = charges.capture_transaction_id
  ) corrections ON TRUE
  WHERE charges.refunded_amount < 0 OR charges.refunded_amount > charges.amount
    OR charges.refunded_amount <> COALESCE(refunds.amount, 0) - COALESCE(corrections.amount, 0);

  INSERT INTO reconciliation_scan
  SELECT 'illegal_reference_chain', 'critical', 'high', transactions.billing_account_id, 'transaction', transactions.transaction_id,
    jsonb_build_object('command_type', transactions.command_type, 'related_transaction_id', transactions.related_transaction_id,
      'related_command_type', related.command_type, 'related_billing_account_id', related.billing_account_id)
  FROM credit_transactions transactions
  LEFT JOIN credit_transactions related ON related.transaction_id = transactions.related_transaction_id
  WHERE (transactions.command_type IN ('grant', 'deduction', 'hold', 'purchase_credit', 'payment_dispute_debit') AND transactions.related_transaction_id IS NOT NULL)
    OR (transactions.command_type IN ('capture', 'release') AND
      (related.transaction_id IS NULL OR related.command_type <> 'hold' OR related.billing_account_id <> transactions.billing_account_id))
    OR (transactions.command_type = 'refund' AND
      (related.transaction_id IS NULL OR related.command_type <> 'capture' OR related.billing_account_id <> transactions.billing_account_id))
    OR (transactions.command_type = 'purchase_refund' AND
      (related.transaction_id IS NULL OR related.command_type <> 'hold' OR related.billing_account_id <> transactions.billing_account_id))
    OR (transactions.command_type = 'payment_dispute_restore' AND
      (related.transaction_id IS NULL OR related.command_type <> 'payment_dispute_debit' OR related.billing_account_id <> transactions.billing_account_id
       OR related.business_reference_type <> transactions.business_reference_type
       OR related.business_reference_id <> transactions.business_reference_id))
    OR (transactions.command_type = 'correction' AND
      (related.transaction_id IS NULL OR related.command_type NOT IN ('grant', 'deduction', 'refund')
       OR related.actor_type <> 'admin' OR related.billing_account_id <> transactions.billing_account_id));

  INSERT INTO reconciliation_scan
  SELECT 'payment_dispute_restore_exceeds_debit', 'critical', 'high', debit.billing_account_id,
    'transaction', debit.transaction_id,
    jsonb_build_object('debited_amount', debit.command_amount, 'restored_amount', COALESCE(restores.amount, 0))
  FROM credit_transactions debit
  LEFT JOIN LATERAL (
    SELECT SUM(command_amount) AS amount
    FROM credit_transactions restore
    WHERE restore.command_type = 'payment_dispute_restore'
      AND restore.related_transaction_id = debit.transaction_id
  ) restores ON TRUE
  WHERE debit.command_type = 'payment_dispute_debit'
    AND COALESCE(restores.amount, 0) > debit.command_amount;

  INSERT INTO reconciliation_scan
  SELECT 'hold_state_invalid', 'critical', 'high', holds.billing_account_id, 'hold', holds.hold_transaction_id,
    jsonb_build_object('hold_status', holds.status, 'hold_amount', holds.amount, 'hold_command', hold_tx.command_type,
      'capture_command', capture_tx.command_type, 'release_command', release_tx.command_type)
  FROM credit_holds holds
  LEFT JOIN credit_transactions hold_tx ON hold_tx.transaction_id = holds.hold_transaction_id
  LEFT JOIN credit_transactions capture_tx ON capture_tx.transaction_id = holds.capture_transaction_id
  LEFT JOIN credit_transactions release_tx ON release_tx.transaction_id = holds.release_transaction_id
  WHERE hold_tx.command_type IS DISTINCT FROM 'hold' OR hold_tx.billing_account_id IS DISTINCT FROM holds.billing_account_id
    OR hold_tx.command_amount IS DISTINCT FROM holds.amount
    OR (holds.status = 'held' AND (
      holds.captured_amount + holds.released_amount >= holds.amount
      OR holds.settled_at IS NOT NULL
    ))
    OR (holds.status = 'captured' AND (
      holds.captured_amount <= 0
      OR holds.captured_amount + holds.released_amount <> holds.amount
      OR holds.settled_at IS NULL
      OR capture_tx.command_type NOT IN ('capture', 'purchase_refund')
      OR capture_tx.command_amount IS DISTINCT FROM holds.captured_amount
      OR capture_tx.billing_account_id IS DISTINCT FROM holds.billing_account_id
      OR capture_tx.related_transaction_id IS DISTINCT FROM holds.hold_transaction_id
      OR (holds.released_amount > 0 AND (
        release_tx.command_type IS DISTINCT FROM 'release'
        OR release_tx.command_amount IS DISTINCT FROM holds.released_amount
        OR release_tx.billing_account_id IS DISTINCT FROM holds.billing_account_id
        OR release_tx.related_transaction_id IS DISTINCT FROM holds.hold_transaction_id
      ))
    ))
    OR (holds.status = 'released' AND (
      holds.captured_amount <> 0
      OR holds.released_amount <> holds.amount
      OR holds.settled_at IS NULL
      OR release_tx.command_type IS DISTINCT FROM 'release'
      OR release_tx.command_amount IS DISTINCT FROM holds.released_amount
      OR release_tx.billing_account_id IS DISTINCT FROM holds.billing_account_id
      OR release_tx.related_transaction_id IS DISTINCT FROM holds.hold_transaction_id
    ));

  INSERT INTO reconciliation_scan
  SELECT 'hold_terminal_transaction_count', 'critical', 'high', holds.billing_account_id, 'hold', holds.hold_transaction_id,
    jsonb_build_object('hold_status', holds.status, 'settlement_transaction_count', COUNT(terminals.transaction_id))
  FROM credit_holds holds
  LEFT JOIN credit_transactions terminals ON terminals.related_transaction_id = holds.hold_transaction_id
    AND terminals.command_type IN ('capture', 'release', 'purchase_refund')
  GROUP BY holds.hold_transaction_id, holds.billing_account_id, holds.status, holds.captured_amount, holds.released_amount
  HAVING COUNT(terminals.transaction_id) <>
    (CASE WHEN holds.captured_amount > 0 THEN 1 ELSE 0 END) +
    (CASE WHEN holds.released_amount > 0 THEN 1 ELSE 0 END);

  INSERT INTO reconciliation_scan
  SELECT 'charge_state_invalid', 'critical', 'high', charges.billing_account_id, 'charge', charges.capture_transaction_id,
    jsonb_build_object('charge_amount', charges.amount, 'capture_command', capture.command_type, 'capture_amount', capture.command_amount)
  FROM credit_charges charges
  LEFT JOIN credit_transactions capture ON capture.transaction_id = charges.capture_transaction_id
  WHERE capture.command_type IS DISTINCT FROM 'capture' OR capture.billing_account_id IS DISTINCT FROM charges.billing_account_id
    OR capture.command_amount IS DISTINCT FROM charges.amount;

  WITH normalized AS (
    SELECT scan.*, encode(sha256(convert_to(concat_ws(E'\x1f', finding_type, COALESCE(billing_account_id::TEXT, ''), subject_type, subject_id), 'UTF8')), 'hex') AS fingerprint
    FROM reconciliation_scan scan WHERE finding_type IS NOT NULL
  ), upserted AS (
    INSERT INTO credit_reconciliation_findings (
      fingerprint, finding_type, severity, risk, billing_account_id, subject_type, subject_id, details,
      first_seen_run_id, last_seen_run_id
    ) SELECT fingerprint, finding_type, severity, risk, billing_account_id, subject_type, subject_id, details, p_run_id, p_run_id
      FROM normalized
    ON CONFLICT (fingerprint) DO UPDATE SET
      severity = EXCLUDED.severity, risk = EXCLUDED.risk, billing_account_id = EXCLUDED.billing_account_id,
      details = EXCLUDED.details, status = 'open', isolated = FALSE, last_seen_run_id = p_run_id,
      last_seen_at = clock_timestamp(), occurrence_count = credit_reconciliation_findings.occurrence_count + 1,
      resolved_at = NULL
    RETURNING id
  ) INSERT INTO credit_reconciliation_run_findings (run_id, finding_id)
    SELECT p_run_id, id FROM upserted ON CONFLICT DO NOTHING;

  UPDATE credit_reconciliation_findings
  SET status = 'resolved', resolved_at = clock_timestamp()
  WHERE status = 'open' AND last_seen_run_id <> p_run_id;

  FOR v_finding IN
    SELECT restrictions.restriction_id::TEXT AS restriction_id,
           restrictions.billing_account_id,
           findings.id AS finding_id
    FROM credit_wallet_restrictions restrictions
    JOIN credit_reconciliation_findings findings
      ON restrictions.related_object_type = 'finding'
     AND restrictions.related_object_id = findings.id::TEXT
    WHERE restrictions.restriction_type = 'ledger_integrity'
      AND restrictions.source = 'reconciliation'
      AND restrictions.released_at IS NULL
      AND (
        findings.status <> 'open'
        OR findings.risk <> 'high'
        OR findings.billing_account_id IS NULL
        OR findings.billing_account_id <> restrictions.billing_account_id
        OR findings.last_seen_run_id <> p_run_id
      )
    ORDER BY restrictions.billing_account_id, findings.id
  LOOP
    PERFORM release_credit_wallet_restriction(
      v_finding.restriction_id,
      'system',
      'credits-reconciliation',
      'finding no longer requires ledger integrity isolation'
    );
  END LOOP;

  FOR v_finding IN
    SELECT findings.id, findings.billing_account_id, findings.finding_type
    FROM credit_reconciliation_findings findings
    WHERE findings.last_seen_run_id = p_run_id
      AND findings.status = 'open'
      AND findings.risk = 'high'
      AND findings.billing_account_id IS NOT NULL
    ORDER BY findings.billing_account_id, findings.id
  LOOP
    SELECT restriction_id INTO v_restriction_id
    FROM create_credit_wallet_restriction(
      v_finding.billing_account_id,
      'ledger_integrity',
      'reconciliation',
      'finding',
      v_finding.id::TEXT,
      'high-risk reconciliation finding: ' || v_finding.finding_type,
      'system',
      'credits-reconciliation'
    );
  END LOOP;

  UPDATE credit_reconciliation_findings findings
  SET isolated = EXISTS (
    SELECT 1
    FROM credit_wallet_restrictions restrictions
    WHERE restrictions.billing_account_id = findings.billing_account_id
      AND restrictions.restriction_type = 'ledger_integrity'
      AND restrictions.source = 'reconciliation'
      AND restrictions.related_object_type = 'finding'
      AND restrictions.related_object_id = findings.id::TEXT
      AND restrictions.released_at IS NULL
  );

  UPDATE credit_reconciliation_run_findings links SET
    observed_severity = findings.severity,
    observed_risk = findings.risk,
    observed_details = findings.details,
    observed_status = findings.status,
    observed_isolated = findings.isolated,
    observed_occurrence_count = findings.occurrence_count,
    observed_first_seen_at = findings.first_seen_at,
    observed_last_seen_at = findings.last_seen_at,
    observed_resolved_at = findings.resolved_at
  FROM credit_reconciliation_findings findings
  WHERE links.run_id = p_run_id AND links.finding_id = findings.id;

  UPDATE credit_reconciliation_runs runs SET status = 'completed', completed_at = clock_timestamp(),
    finding_count = (SELECT COUNT(*) FROM credit_reconciliation_run_findings WHERE run_id = p_run_id),
    critical_count = (SELECT COUNT(*) FROM credit_reconciliation_findings WHERE last_seen_run_id = p_run_id AND severity = 'critical'),
    isolated_wallet_count = (SELECT COUNT(DISTINCT billing_account_id) FROM credit_reconciliation_findings
      WHERE last_seen_run_id = p_run_id AND isolated)
  WHERE runs.id = p_run_id;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog;

CREATE OR REPLACE FUNCTION guard_credit_account_projection()
RETURNS trigger AS $$
BEGIN
  IF NEW.balance IS DISTINCT FROM OLD.balance
     AND pg_trigger_depth() < 2 THEN
    RAISE EXCEPTION 'credit account balances are ledger projections and cannot be overwritten directly';
  END IF;
  IF NEW.billing_account_id IS DISTINCT FROM OLD.billing_account_id
     OR NEW.wallet_user_id IS DISTINCT FROM OLD.wallet_user_id
     OR NEW.asset_code IS DISTINCT FROM OLD.asset_code
     OR NEW.account_role IS DISTINCT FROM OLD.account_role
     OR NEW.shard_key IS DISTINCT FROM OLD.shard_key THEN
    RAISE EXCEPTION 'credit account identity is immutable';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION ensure_credit_wallet_projection_on_user_insert() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  PERFORM provision_billing_account_wallet(ensure_personal_billing_account_for_user(NEW.id));
  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION guard_task_billing_settlement_transition() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
  snapshot_total BIGINT;
  snapshot_unit BIGINT;
  snapshot_quantity BIGINT;
  snapshot_user BIGINT;
  snapshot_account BIGINT;
  snapshot_hold TEXT;
  valid_terminal BOOLEAN;
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB202', MESSAGE = 'task billing settlements cannot be deleted';
  END IF;
  SELECT total_credits, unit_credits, quantity, user_id, billing_account_id, hold_transaction_id
  INTO snapshot_total, snapshot_unit, snapshot_quantity, snapshot_user, snapshot_account, snapshot_hold
  FROM task_billing_snapshots
  WHERE task_id = NEW.task_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB205', MESSAGE = 'task billing settlement snapshot is missing';
  END IF;
  IF TG_OP = 'INSERT' THEN
    IF (snapshot_total = 0 AND NEW.state <> 'free')
       OR (snapshot_total > 0 AND NEW.state <> 'held') THEN
      RAISE EXCEPTION USING ERRCODE = 'IB206', MESSAGE = 'task billing initial settlement state does not match snapshot';
    END IF;
    RETURN NEW;
  END IF;
  IF NEW.task_id <> OLD.task_id
     OR OLD.state <> 'held'
     OR NEW.state NOT IN ('captured', 'released')
     OR NEW.actual_output_count < 0
     OR NEW.actual_output_count > snapshot_quantity
     OR NEW.captured_credits <> NEW.actual_output_count * snapshot_unit
     OR NEW.released_credits <> snapshot_total - NEW.captured_credits
     OR (NEW.captured_credits > 0) <> (NEW.capture_transaction_id IS NOT NULL)
     OR (NEW.released_credits > 0) <> (NEW.release_transaction_id IS NOT NULL)
     OR NEW.settled_at IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = 'IB203', MESSAGE = 'invalid task billing settlement transition';
  END IF;
  SELECT (
    (NEW.capture_transaction_id IS NULL OR EXISTS (
      SELECT 1 FROM credit_transactions transactions
      WHERE transactions.transaction_id = NEW.capture_transaction_id
        AND transactions.command_type = 'capture'
        AND transactions.command_amount = NEW.captured_credits
        AND transactions.billing_account_id = snapshot_account
        AND transactions.related_transaction_id = snapshot_hold
        AND transactions.business_reference_type = 'task'
        AND transactions.business_reference_id = NEW.task_id
    )) AND
    (NEW.release_transaction_id IS NULL OR EXISTS (
      SELECT 1 FROM credit_transactions transactions
      WHERE transactions.transaction_id = NEW.release_transaction_id
        AND transactions.command_type = 'release'
        AND transactions.command_amount = NEW.released_credits
        AND transactions.billing_account_id = snapshot_account
        AND transactions.related_transaction_id = snapshot_hold
        AND transactions.business_reference_type = 'task'
        AND transactions.business_reference_id = NEW.task_id
    ))
  ) INTO valid_terminal;
  IF NOT valid_terminal THEN
    RAISE EXCEPTION USING ERRCODE = 'IB207', MESSAGE = 'task billing terminal settlement does not match Credit Ledger';
  END IF;
  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION validate_task_billing_snapshot_insert() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
  valid_hold BOOLEAN;
BEGIN
  IF NEW.total_credits = 0 THEN
    RETURN NEW;
  END IF;
  SELECT EXISTS (
    SELECT 1
    FROM credit_transactions transactions
    WHERE transactions.transaction_id = NEW.hold_transaction_id
      AND transactions.command_type = 'hold'
      AND transactions.billing_account_id = NEW.billing_account_id
      AND transactions.command_amount = NEW.total_credits
      AND transactions.business_reference_type = 'task'
      AND transactions.business_reference_id = NEW.task_id
  ) INTO valid_hold;
  IF NOT valid_hold THEN
    RAISE EXCEPTION USING ERRCODE = 'IB204', MESSAGE = 'task billing snapshot Hold does not match Credit Ledger';
  END IF;
  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION guard_payment_order_lifecycle()
RETURNS TRIGGER AS $$
BEGIN
  IF OLD.id <> NEW.id
     OR OLD.public_order_number <> NEW.public_order_number
     OR OLD.public_token <> NEW.public_token
     OR OLD.user_id <> NEW.user_id
     OR OLD.client_idempotency_key <> NEW.client_idempotency_key
     OR OLD.request_hash <> NEW.request_hash
     OR OLD.stripe_account_id <> NEW.stripe_account_id
     OR OLD.stripe_product_id <> NEW.stripe_product_id
     OR OLD.idea_quantity <> NEW.idea_quantity
     OR OLD.base_currency <> NEW.base_currency
     OR OLD.base_amount_minor <> NEW.base_amount_minor
     OR OLD.terms_version <> NEW.terms_version
     OR OLD.refund_policy_version <> NEW.refund_policy_version
     OR OLD.return_base_url <> NEW.return_base_url
     OR OLD.created_at <> NEW.created_at THEN
    RAISE EXCEPTION 'payment order identity is immutable';
  END IF;
  IF OLD.stripe_customer_id <> '' AND OLD.stripe_customer_id <> NEW.stripe_customer_id THEN
    RAISE EXCEPTION 'payment order customer is immutable once assigned';
  END IF;
  IF OLD.stripe_checkout_session_id IS NOT NULL
     AND OLD.stripe_checkout_session_id IS DISTINCT FROM NEW.stripe_checkout_session_id THEN
    RAISE EXCEPTION 'payment order Checkout Session is immutable once assigned';
  END IF;
  IF OLD.checkout_url <> '' AND NEW.checkout_url <> '' AND OLD.checkout_url <> NEW.checkout_url THEN
    RAISE EXCEPTION 'payment order Checkout URL is immutable once assigned';
  END IF;
  IF OLD.stripe_checkout_session_id IS NOT NULL AND OLD.expires_at <> NEW.expires_at THEN
    RAISE EXCEPTION 'payment order Checkout expiry is immutable once assigned';
  END IF;
  IF OLD.status IN ('paid_pending_credit', 'credited') AND (
    OLD.stripe_payment_intent_id <> NEW.stripe_payment_intent_id
    OR OLD.stripe_charge_id <> NEW.stripe_charge_id
    OR OLD.paid_subtotal_minor IS DISTINCT FROM NEW.paid_subtotal_minor
    OR OLD.paid_tax_minor IS DISTINCT FROM NEW.paid_tax_minor
    OR OLD.paid_total_minor IS DISTINCT FROM NEW.paid_total_minor
    OR OLD.paid_currency <> NEW.paid_currency
    OR OLD.receipt_url <> NEW.receipt_url
    OR OLD.paid_at IS DISTINCT FROM NEW.paid_at
  ) THEN
    RAISE EXCEPTION 'paid payment order facts are immutable';
  END IF;
  IF OLD.status = 'credited' AND (
    OLD.credit_transaction_id IS DISTINCT FROM NEW.credit_transaction_id
    OR OLD.credited_at IS DISTINCT FROM NEW.credited_at
  ) THEN
    RAISE EXCEPTION 'credited payment order facts are immutable';
  END IF;
  IF OLD.status IN ('cancelled', 'expired', 'payment_failed') AND NEW.status <> 'paid_pending_credit' AND (
    OLD.stripe_customer_id <> NEW.stripe_customer_id
    OR OLD.stripe_checkout_session_id IS DISTINCT FROM NEW.stripe_checkout_session_id
    OR OLD.checkout_url <> NEW.checkout_url
    OR OLD.stripe_payment_intent_id <> NEW.stripe_payment_intent_id
    OR OLD.stripe_charge_id <> NEW.stripe_charge_id
    OR OLD.paid_subtotal_minor IS DISTINCT FROM NEW.paid_subtotal_minor
    OR OLD.paid_tax_minor IS DISTINCT FROM NEW.paid_tax_minor
    OR OLD.paid_total_minor IS DISTINCT FROM NEW.paid_total_minor
    OR OLD.paid_currency <> NEW.paid_currency
    OR OLD.receipt_url <> NEW.receipt_url
    OR OLD.credit_transaction_id IS DISTINCT FROM NEW.credit_transaction_id
    OR OLD.paid_at IS DISTINCT FROM NEW.paid_at
    OR OLD.credited_at IS DISTINCT FROM NEW.credited_at
  ) THEN
    RAISE EXCEPTION 'terminal unpaid payment order facts are immutable';
  END IF;
  IF OLD.status <> NEW.status AND NOT (
    (OLD.status = 'creating' AND NEW.status IN ('checkout_open', 'paid_pending_credit', 'expired', 'payment_failed'))
    OR (OLD.status = 'checkout_open' AND NEW.status IN ('async_payment_pending', 'paid_pending_credit', 'cancelled', 'expired', 'payment_failed'))
    OR (OLD.status = 'async_payment_pending' AND NEW.status IN ('paid_pending_credit', 'expired', 'payment_failed'))
    OR (OLD.status = 'paid_pending_credit' AND NEW.status = 'credited')
    OR (OLD.status IN ('cancelled', 'expired', 'payment_failed') AND NEW.status = 'paid_pending_credit')
  ) THEN
    RAISE EXCEPTION 'invalid payment order state transition: % -> %', OLD.status, NEW.status;
  END IF;
  IF NEW.status = 'credited' AND OLD.status <> 'credited' AND NOT EXISTS (
    SELECT 1
    FROM credit_transactions transaction_fact
    WHERE transaction_fact.transaction_id = NEW.credit_transaction_id
      AND transaction_fact.command_type = 'purchase_credit'
      AND transaction_fact.billing_account_id = ensure_personal_billing_account_for_user(NEW.user_id)
      AND transaction_fact.command_amount = NEW.idea_quantity
      AND transaction_fact.actor_type = 'service'
      AND transaction_fact.actor_id = 'payments'
      AND transaction_fact.source = 'stripe_purchase'
      AND transaction_fact.business_reference_type = 'payment_order'
      AND transaction_fact.business_reference_id = NEW.id::TEXT
  ) THEN
    RAISE EXCEPTION 'credited payment order requires its matching purchase Credit transaction';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;


-- 12. 特权例程绑定安装 schema(与 bootstrap hardening 块同语义):所有
-- SECURITY DEFINER 例程的 search_path 收敛到安装 schema,PUBLIC 无执行权。
-- 快照侧由 bootstrap 块覆盖全部例程;迁移侧在此补齐本票新增/替换的例程,
-- 使语义校验两侧一致。运行时 EXECUTE 授权由 finalize-migrate 按
-- runtime privilege policy 授予,不经 PUBLIC。
DO $$
DECLARE
  routine RECORD;
  target_schema TEXT := current_schema();
BEGIN
  FOR routine IN
    SELECT
      schemas.nspname AS schema_name,
      functions.proname AS function_name,
      pg_catalog.pg_get_function_identity_arguments(functions.oid) AS arguments
    FROM pg_catalog.pg_proc AS functions
    JOIN pg_catalog.pg_namespace AS schemas ON schemas.oid = functions.pronamespace
    WHERE schemas.nspname = target_schema
      AND functions.prosecdef
  LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%I(%s) SET search_path TO %I, pg_catalog, pg_temp',
      routine.schema_name,
      routine.function_name,
      routine.arguments,
      target_schema
    );
    EXECUTE format(
      'REVOKE ALL ON FUNCTION %I.%I(%s) FROM PUBLIC',
      routine.schema_name,
      routine.function_name,
      routine.arguments
    );
  END LOOP;
END;
$$;
