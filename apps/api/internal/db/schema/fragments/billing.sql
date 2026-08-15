-- Canonical objects introduced by the clean-break baseline.
-- Name: delete_archived_billing_item(text, bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION delete_archived_billing_item(p_sku text, p_actor_id bigint) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $_$
DECLARE
  v_display_name TEXT;
  v_description TEXT;
  v_price_count BIGINT;
BEGIN
  p_sku := lower(btrim(p_sku));
  IF p_sku IS NULL
     OR p_sku !~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'
     OR char_length(p_sku) > 128
     OR p_actor_id IS NULL
     OR p_actor_id <= 0 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid archived billing item delete command';
  END IF;

  SELECT skus.display_name, skus.description
  INTO v_display_name, v_description
  FROM billing_skus skus
  WHERE skus.sku = p_sku
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'IB103', MESSAGE = 'billing SKU not found';
  END IF;

  -- The row lock serializes this check with concurrent snapshot FK acquisition.
  IF EXISTS (SELECT 1 FROM task_billing_snapshots snapshots WHERE snapshots.sku = p_sku) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB108', MESSAGE = 'billing item has durable references';
  END IF;

  SELECT count(*) INTO v_price_count
  FROM billing_price_versions prices
  WHERE prices.sku = p_sku;

  INSERT INTO billing_admin_actions (
    action_type, sku, price_version_id, actor_id, reason, metadata
  ) VALUES (
    'delete_archived', p_sku, NULL, p_actor_id, 'archived item deleted',
    jsonb_build_object(
      'display_name', v_display_name,
      'description', v_description,
      'price_version_count', v_price_count,
      'outcome', 'deleted'
    )
  );

  PERFORM set_config('ideaboard.billing_archive_delete', p_sku, TRUE);
  DELETE FROM billing_current_prices WHERE sku = p_sku;
  UPDATE billing_skus SET current_price_version_id = NULL WHERE sku = p_sku;
  DELETE FROM billing_price_versions WHERE sku = p_sku;
  DELETE FROM billing_skus WHERE sku = p_sku;
  RETURN p_sku;
END;
$_$;


--
-- Name: guard_task_billing_settlement_transition(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION guard_task_billing_settlement_transition() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
  snapshot_total BIGINT;
  snapshot_unit BIGINT;
  snapshot_quantity BIGINT;
  snapshot_user BIGINT;
  snapshot_hold TEXT;
  valid_terminal BOOLEAN;
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB202', MESSAGE = 'task billing settlements cannot be deleted';
  END IF;
  SELECT total_credits, unit_credits, quantity, user_id, hold_transaction_id
  INTO snapshot_total, snapshot_unit, snapshot_quantity, snapshot_user, snapshot_hold
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
        AND transactions.wallet_user_id = snapshot_user
        AND transactions.related_transaction_id = snapshot_hold
        AND transactions.business_reference_type = 'task'
        AND transactions.business_reference_id = NEW.task_id
    )) AND
    (NEW.release_transaction_id IS NULL OR EXISTS (
      SELECT 1 FROM credit_transactions transactions
      WHERE transactions.transaction_id = NEW.release_transaction_id
        AND transactions.command_type = 'release'
        AND transactions.command_amount = NEW.released_credits
        AND transactions.wallet_user_id = snapshot_user
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


--
-- Name: guard_task_submission_claim(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION guard_task_submission_claim() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
  valid_claim BOOLEAN;
BEGIN
  IF TG_OP <> 'INSERT' THEN
    RAISE EXCEPTION USING ERRCODE = 'IB208', MESSAGE = 'task submission claims are immutable';
  END IF;
  SELECT EXISTS (
    SELECT 1
    FROM task_billing_snapshots snapshots
    JOIN tasks ON tasks.task_id = snapshots.task_id
    WHERE snapshots.task_id = NEW.task_id
      AND snapshots.user_id = NEW.user_id
      AND tasks.user_id = NEW.user_id
      AND tasks.client_action_id = NEW.client_action_id
  ) INTO valid_claim;
  IF NOT valid_claim THEN
    RAISE EXCEPTION USING ERRCODE = 'IB209', MESSAGE = 'task submission claim does not match Task Billing facts';
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: prevent_billing_admin_action_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_billing_admin_action_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = 'IB107', MESSAGE = 'billing admin actions are append-only';
END;
$$;


--
-- Name: prevent_billing_price_version_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_billing_price_version_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN
  IF TG_OP = 'DELETE'
     AND current_setting('ideaboard.billing_archive_delete', TRUE) = OLD.sku THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION USING ERRCODE = 'IB105', MESSAGE = 'billing price versions are immutable';
END;
$$;


--
-- Name: prevent_billing_sku_identity_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_billing_sku_identity_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    IF current_setting('ideaboard.billing_archive_delete', TRUE) = OLD.sku THEN
      RETURN OLD;
    END IF;
    RAISE EXCEPTION USING ERRCODE = 'IB104', MESSAGE = 'billing SKU cannot be deleted or reused';
  END IF;
  IF NEW.sku <> OLD.sku THEN
    RAISE EXCEPTION USING ERRCODE = 'IB104', MESSAGE = 'billing SKU cannot be renamed';
  END IF;
  RETURN NEW;
END;
$$;


--
-- Name: prevent_task_billing_snapshot_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION prevent_task_billing_snapshot_mutation() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $$
BEGIN
  RAISE EXCEPTION USING ERRCODE = 'IB201', MESSAGE = 'task billing snapshots are immutable';
END;
$$;


--
-- Name: publish_billing_price(text, text, text, bigint, bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION publish_billing_price(p_sku text, p_display_name text, p_description text, p_unit_credits bigint, p_actor_id bigint) RETURNS TABLE(price_version_id bigint, version bigint, unit_credits bigint, created_by bigint, created_at timestamp with time zone)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $_$
DECLARE
  v_old_price_version_id BIGINT;
  v_old_version BIGINT;
  v_old_unit_credits BIGINT;
  v_version BIGINT;
  v_price_version_id BIGINT;
  v_materialized BOOLEAN;
  v_inserted BIGINT;
BEGIN
  p_sku := lower(btrim(p_sku));
  p_display_name := btrim(p_display_name);
  p_description := btrim(COALESCE(p_description, ''));
  IF p_sku IS NULL
     OR p_sku !~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'
     OR char_length(p_sku) > 128
     OR p_display_name IS NULL
     OR char_length(p_display_name) NOT BETWEEN 1 AND 120
     OR char_length(p_description) > 1000
     OR p_unit_credits IS NULL
     OR p_unit_credits < 0
     OR p_unit_credits > 9000000000000000
     OR p_actor_id IS NULL
     OR p_actor_id <= 0 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing price publish command';
  END IF;

  INSERT INTO billing_skus (
    sku, display_name, description, enabled, created_by, updated_by
  ) VALUES (
    p_sku, p_display_name, p_description, TRUE, p_actor_id, p_actor_id
  ) ON CONFLICT (sku) DO NOTHING;
  GET DIAGNOSTICS v_inserted = ROW_COUNT;
  v_materialized := v_inserted > 0;

  SELECT skus.current_price_version_id, prices.version, prices.unit_credits
  INTO v_old_price_version_id, v_old_version, v_old_unit_credits
  FROM billing_skus skus
  LEFT JOIN billing_price_versions prices
    ON prices.sku = skus.sku
   AND prices.price_version_id = skus.current_price_version_id
  WHERE skus.sku = p_sku
  FOR UPDATE OF skus;

  UPDATE billing_skus
  SET display_name = p_display_name,
      description = p_description,
      enabled = CASE WHEN v_old_price_version_id IS NULL THEN TRUE ELSE enabled END,
      updated_by = p_actor_id,
      updated_at = clock_timestamp()
  WHERE sku = p_sku;

  SELECT COALESCE(max(prices.version), 0) + 1
  INTO v_version
  FROM billing_price_versions prices
  WHERE prices.sku = p_sku;

  INSERT INTO billing_price_versions (sku, version, unit_credits, created_by)
  VALUES (p_sku, v_version, p_unit_credits, p_actor_id)
  RETURNING billing_price_versions.price_version_id INTO v_price_version_id;

  UPDATE billing_skus
  SET current_price_version_id = v_price_version_id,
      updated_by = p_actor_id,
      updated_at = clock_timestamp()
  WHERE sku = p_sku;

  INSERT INTO billing_admin_actions (
    action_type, sku, price_version_id, actor_id, reason, metadata
  ) VALUES (
    'create_price', p_sku, v_price_version_id, p_actor_id, 'price published',
    jsonb_build_object(
      'materialized', v_materialized,
      'old', CASE
        WHEN v_old_price_version_id IS NULL THEN NULL
        ELSE jsonb_build_object(
          'price_version_id', v_old_price_version_id,
          'version', v_old_version,
          'unit_credits', v_old_unit_credits
        )
      END,
      'new', jsonb_build_object(
        'price_version_id', v_price_version_id,
        'version', v_version,
        'unit_credits', p_unit_credits
      ),
      'outcome', 'published'
    )
  );

  RETURN QUERY
  SELECT prices.price_version_id, prices.version, prices.unit_credits,
         prices.created_by, prices.created_at
  FROM billing_price_versions prices
  WHERE prices.price_version_id = v_price_version_id;
END;
$_$;


--
-- Name: set_billing_item_availability(text, boolean, bigint); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION set_billing_item_availability(p_sku text, p_enabled boolean, p_actor_id bigint) RETURNS TABLE(sku text, enabled boolean, updated_at timestamp with time zone)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public', 'pg_catalog', 'pg_temp'
    AS $_$
DECLARE
  v_old_enabled BOOLEAN;
  v_price_version_id BIGINT;
  v_updated_at TIMESTAMPTZ(3);
  v_outcome TEXT;
BEGIN
  p_sku := lower(btrim(p_sku));
  IF p_sku IS NULL
     OR p_sku !~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'
     OR char_length(p_sku) > 128
     OR p_enabled IS NULL
     OR p_actor_id IS NULL
     OR p_actor_id <= 0 THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing availability command';
  END IF;

  SELECT skus.enabled, skus.current_price_version_id, skus.updated_at
  INTO v_old_enabled, v_price_version_id, v_updated_at
  FROM billing_skus skus
  WHERE skus.sku = p_sku
  FOR UPDATE;

  IF NOT FOUND OR v_price_version_id IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = 'IB104', MESSAGE = 'billing item has no published price';
  END IF;

  IF v_old_enabled IS DISTINCT FROM p_enabled THEN
    v_updated_at := clock_timestamp();
    UPDATE billing_skus
    SET enabled = p_enabled,
        updated_by = p_actor_id,
        updated_at = v_updated_at
    WHERE billing_skus.sku = p_sku;
    v_outcome := 'changed';
  ELSE
    v_outcome := 'unchanged';
  END IF;

  INSERT INTO billing_admin_actions (
    action_type, sku, price_version_id, actor_id, reason, metadata
  ) VALUES (
    'set_availability', p_sku, v_price_version_id, p_actor_id, 'availability updated',
    jsonb_build_object(
      'old', CASE WHEN v_old_enabled THEN 'active' ELSE 'disabled' END,
      'new', CASE WHEN p_enabled THEN 'active' ELSE 'disabled' END,
      'outcome', v_outcome
    )
  );

  RETURN QUERY SELECT p_sku, p_enabled, v_updated_at;
END;
$_$;


--
-- Name: validate_task_billing_snapshot_insert(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION validate_task_billing_snapshot_insert() RETURNS trigger
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
      AND transactions.wallet_user_id = NEW.user_id
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




--

--
--






--
-- Name: billing_admin_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE billing_admin_actions (
    id bigint NOT NULL,
    action_type character varying(32) NOT NULL,
    sku character varying(128) NOT NULL,
    price_version_id bigint,
    actor_id bigint NOT NULL,
    reason character varying(1000) NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT billing_admin_actions_actor_id_check CHECK ((actor_id > 0)),
    CONSTRAINT ck_billing_admin_actions_metadata CHECK ((jsonb_typeof(metadata) = 'object'::text)),
    CONSTRAINT ck_billing_admin_actions_price_scope CHECK (((((action_type)::text = ANY (ARRAY[('create_sku'::character varying)::text, ('update_sku'::character varying)::text, ('delete_archived'::character varying)::text])) AND (price_version_id IS NULL)) OR (((action_type)::text = ANY (ARRAY[('create_price'::character varying)::text, ('set_price_enabled'::character varying)::text, ('set_availability'::character varying)::text])) AND (price_version_id IS NOT NULL)))),
    CONSTRAINT ck_billing_admin_actions_reason CHECK ((((reason)::text = btrim((reason)::text, '
'::text)) AND ((char_length((reason)::text) >= 1) AND (char_length((reason)::text) <= 1000)))),
    CONSTRAINT ck_billing_admin_actions_type CHECK (((action_type)::text = ANY (ARRAY[('create_sku'::character varying)::text, ('update_sku'::character varying)::text, ('create_price'::character varying)::text, ('set_price_enabled'::character varying)::text, ('set_availability'::character varying)::text, ('delete_archived'::character varying)::text])))
);

-- The imported dump lost the leading space and tab in the trim character set.
-- Rebind the constraint to the canonical whitespace contract.
ALTER TABLE billing_admin_actions
  DROP CONSTRAINT ck_billing_admin_actions_reason;
ALTER TABLE billing_admin_actions
  ADD CONSTRAINT ck_billing_admin_actions_reason CHECK (
    reason = btrim(reason, E' \t\n\r\f')
    AND char_length(reason) BETWEEN 1 AND 1000
  );


--
-- Name: billing_admin_actions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE billing_admin_actions ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME billing_admin_actions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: billing_price_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE billing_price_versions (
    price_version_id bigint NOT NULL,
    sku character varying(128) NOT NULL,
    output_resolution character varying(8) DEFAULT '1k'::character varying NOT NULL,
    version bigint NOT NULL,
    unit_credits bigint NOT NULL,
    created_by bigint NOT NULL,
    created_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT billing_price_versions_created_by_check CHECK ((created_by > 0)),
    CONSTRAINT billing_price_versions_output_resolution_check CHECK (((output_resolution)::text = ANY (ARRAY['default'::text, '1k'::text, '2k'::text, '4k'::text]))),
    CONSTRAINT billing_price_versions_unit_credits_check CHECK (((unit_credits >= 0) AND (unit_credits <= '9000000000000000'::bigint))),
    CONSTRAINT billing_price_versions_version_check CHECK ((version > 0))
);


--
-- Name: billing_price_versions_price_version_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE billing_price_versions ALTER COLUMN price_version_id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME billing_price_versions_price_version_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: billing_skus; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE billing_skus (
    sku character varying(128) NOT NULL,
    display_name character varying(120) NOT NULL,
    description character varying(1000) DEFAULT ''::character varying NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    current_price_version_id bigint,
    created_by bigint NOT NULL,
    updated_by bigint NOT NULL,
    created_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT billing_skus_canonical_check CHECK ((((sku)::text = lower(btrim((sku)::text))) AND ((char_length((sku)::text) >= 3) AND (char_length((sku)::text) <= 128)) AND ((sku)::text ~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'::text))),
    CONSTRAINT billing_skus_created_by_check CHECK ((created_by > 0)),
    CONSTRAINT billing_skus_display_name_check CHECK (((char_length(btrim((display_name)::text)) >= 1) AND (char_length(btrim((display_name)::text)) <= 120))),
    CONSTRAINT billing_skus_updated_by_check CHECK ((updated_by > 0))
);

CREATE TABLE billing_current_prices (
    sku character varying(128) NOT NULL,
    output_resolution character varying(8) NOT NULL,
    price_version_id bigint NOT NULL,
    updated_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT billing_current_prices_pkey PRIMARY KEY (sku, output_resolution),
    CONSTRAINT billing_current_prices_resolution_check CHECK (((output_resolution)::text = ANY (ARRAY['default'::text, '1k'::text, '2k'::text, '4k'::text])))
);


--
-- Name: task_billing_settlements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_billing_settlements (
    task_id character varying(64) NOT NULL,
    state character varying(16) NOT NULL,
    actual_output_count bigint NOT NULL DEFAULT 0,
    captured_credits bigint NOT NULL DEFAULT 0,
    released_credits bigint NOT NULL DEFAULT 0,
    capture_transaction_id text,
    release_transaction_id text,
    settled_at timestamp(3) with time zone,
    updated_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT task_billing_settlement_state_check CHECK ((((state)::text = 'held'::text AND capture_transaction_id IS NULL AND release_transaction_id IS NULL AND settled_at IS NULL) OR ((state)::text = 'free'::text AND capture_transaction_id IS NULL AND release_transaction_id IS NULL AND settled_at IS NOT NULL) OR ((state)::text IN ('captured', 'released') AND settled_at IS NOT NULL))),
    CONSTRAINT task_billing_settlement_amounts_check CHECK (actual_output_count >= 0 AND captured_credits >= 0 AND released_credits >= 0),
    CONSTRAINT task_billing_settlements_state_check CHECK (((state)::text = ANY (ARRAY[('held'::character varying)::text, ('free'::character varying)::text, ('captured'::character varying)::text, ('released'::character varying)::text])))
);


--
-- Name: task_billing_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_billing_snapshots (
    task_id character varying(64) NOT NULL,
    user_id bigint NOT NULL,
    feature_slug character varying(64) NOT NULL,
    sku character varying(128) NOT NULL,
    price_version_id bigint NOT NULL,
    price_version bigint NOT NULL,
    unit_credits bigint NOT NULL,
    quantity bigint NOT NULL,
    total_credits bigint NOT NULL,
    quoted_at timestamp(3) with time zone NOT NULL,
    hold_transaction_id text,
    created_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    display_name character varying(120) NOT NULL,
    output_resolution character varying(8) DEFAULT '1k'::character varying NOT NULL,
    CONSTRAINT task_billing_snapshot_display_name_check CHECK (((char_length(btrim((display_name)::text)) >= 1) AND (char_length(btrim((display_name)::text)) <= 120))),
    CONSTRAINT task_billing_snapshot_hold_check CHECK ((((total_credits = 0) AND (hold_transaction_id IS NULL)) OR ((total_credits > 0) AND (hold_transaction_id IS NOT NULL) AND (btrim(hold_transaction_id) <> ''::text)))),
    CONSTRAINT task_billing_snapshot_total_check CHECK ((total_credits = (unit_credits * quantity))),
    CONSTRAINT task_billing_snapshots_price_version_check CHECK ((price_version > 0)),
    CONSTRAINT task_billing_snapshots_output_resolution_check CHECK (((output_resolution)::text = ANY (ARRAY['default'::text, '1k'::text, '2k'::text, '4k'::text]))),
    CONSTRAINT task_billing_snapshots_quantity_check CHECK ((quantity > 0)),
    CONSTRAINT task_billing_snapshots_total_credits_check CHECK (((total_credits >= 0) AND (total_credits <= '9000000000000000'::bigint))),
    CONSTRAINT task_billing_snapshots_unit_credits_check CHECK (((unit_credits >= 0) AND (unit_credits <= '9000000000000000'::bigint))),
    CONSTRAINT task_billing_snapshots_user_id_check CHECK ((user_id > 0))
);


--
-- Name: task_submission_claims; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE task_submission_claims (
    user_id bigint NOT NULL,
    client_action_id character varying(128) NOT NULL,
    task_id character varying(64) NOT NULL,
    request_hash character varying(64) NOT NULL,
    created_at timestamp(3) with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT task_submission_claims_client_action_id_check CHECK ((btrim((client_action_id)::text) <> ''::text)),
    CONSTRAINT task_submission_claims_request_hash_check CHECK (((request_hash)::text ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT task_submission_claims_user_id_check CHECK ((user_id > 0))
);


--
-- Name: billing_admin_actions billing_admin_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_admin_actions
    ADD CONSTRAINT billing_admin_actions_pkey PRIMARY KEY (id);


--
-- Name: billing_price_versions billing_price_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_price_versions
    ADD CONSTRAINT billing_price_versions_pkey PRIMARY KEY (price_version_id);


--
-- Name: billing_price_versions billing_price_versions_sku_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_price_versions
    ADD CONSTRAINT billing_price_versions_sku_id_key UNIQUE (sku, price_version_id);


--
-- Name: billing_price_versions billing_price_versions_sku_version_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_price_versions
    ADD CONSTRAINT billing_price_versions_sku_version_key UNIQUE (sku, output_resolution, version);

ALTER TABLE ONLY billing_price_versions
    ADD CONSTRAINT billing_price_versions_resolution_id_key UNIQUE (sku, output_resolution, price_version_id);


--
-- Name: billing_price_versions billing_price_versions_snapshot_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_price_versions
    ADD CONSTRAINT billing_price_versions_snapshot_key UNIQUE (sku, price_version_id, version, unit_credits);


--
-- Name: billing_skus billing_skus_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_skus
    ADD CONSTRAINT billing_skus_pkey PRIMARY KEY (sku);

ALTER TABLE ONLY billing_current_prices
    ADD CONSTRAINT billing_current_prices_sku_fkey FOREIGN KEY (sku) REFERENCES billing_skus(sku) ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE ONLY billing_current_prices
    ADD CONSTRAINT billing_current_prices_price_fkey FOREIGN KEY (sku, output_resolution, price_version_id) REFERENCES billing_price_versions(sku, output_resolution, price_version_id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: task_billing_settlements task_billing_settlements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_settlements
    ADD CONSTRAINT task_billing_settlements_pkey PRIMARY KEY (task_id);


--
-- Name: task_billing_snapshots task_billing_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_snapshots
    ADD CONSTRAINT task_billing_snapshots_pkey PRIMARY KEY (task_id);


--
-- Name: task_submission_claims task_submission_claims_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_submission_claims
    ADD CONSTRAINT task_submission_claims_pkey PRIMARY KEY (user_id, client_action_id);


--
-- Name: task_submission_claims task_submission_claims_task_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_submission_claims
    ADD CONSTRAINT task_submission_claims_task_id_key UNIQUE (task_id);


--
-- Name: idx_billing_admin_actions_actor_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_billing_admin_actions_actor_id ON billing_admin_actions USING btree (actor_id, id DESC);


--
-- Name: idx_billing_admin_actions_sku_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_billing_admin_actions_sku_id ON billing_admin_actions USING btree (sku, id DESC);

CREATE INDEX billing_current_prices_price_version_idx ON billing_current_prices USING btree (sku, price_version_id);


--
-- Name: idx_task_billing_snapshots_user_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_task_billing_snapshots_user_created ON task_billing_snapshots USING btree (user_id, created_at);


--
-- Name: billing_admin_actions billing_admin_actions_append_only; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER billing_admin_actions_append_only BEFORE DELETE OR UPDATE ON billing_admin_actions FOR EACH ROW EXECUTE FUNCTION prevent_billing_admin_action_mutation();


--
-- Name: billing_price_versions billing_price_versions_append_only; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER billing_price_versions_append_only BEFORE DELETE OR UPDATE ON billing_price_versions FOR EACH ROW EXECUTE FUNCTION prevent_billing_price_version_mutation();


--
-- Name: billing_skus billing_skus_immutable_identity; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER billing_skus_immutable_identity BEFORE DELETE OR UPDATE ON billing_skus FOR EACH ROW EXECUTE FUNCTION prevent_billing_sku_identity_mutation();


--
-- Name: task_billing_settlements task_billing_settlements_state_machine; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER task_billing_settlements_state_machine BEFORE INSERT OR DELETE OR UPDATE ON task_billing_settlements FOR EACH ROW EXECUTE FUNCTION guard_task_billing_settlement_transition();


--
-- Name: task_billing_snapshots task_billing_snapshots_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER task_billing_snapshots_immutable BEFORE DELETE OR UPDATE ON task_billing_snapshots FOR EACH ROW EXECUTE FUNCTION prevent_task_billing_snapshot_mutation();


--
-- Name: task_billing_snapshots task_billing_snapshots_validate_insert; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER task_billing_snapshots_validate_insert BEFORE INSERT ON task_billing_snapshots FOR EACH ROW EXECUTE FUNCTION validate_task_billing_snapshot_insert();


--
-- Name: task_submission_claims task_submission_claims_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER task_submission_claims_guard BEFORE INSERT OR DELETE OR UPDATE ON task_submission_claims FOR EACH ROW EXECUTE FUNCTION guard_task_submission_claim();


--
-- Name: billing_price_versions billing_price_versions_sku_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_price_versions
    ADD CONSTRAINT billing_price_versions_sku_fkey FOREIGN KEY (sku) REFERENCES billing_skus(sku) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: billing_skus billing_skus_current_price_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY billing_skus
    ADD CONSTRAINT billing_skus_current_price_fk FOREIGN KEY (sku, current_price_version_id) REFERENCES billing_price_versions(sku, price_version_id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: task_billing_settlements task_billing_settlements_capture_transaction_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_settlements
    ADD CONSTRAINT task_billing_settlements_capture_transaction_id_fkey FOREIGN KEY (capture_transaction_id) REFERENCES credit_transactions(transaction_id) ON UPDATE RESTRICT ON DELETE RESTRICT;

ALTER TABLE ONLY task_billing_settlements
    ADD CONSTRAINT task_billing_settlements_release_transaction_id_fkey FOREIGN KEY (release_transaction_id) REFERENCES credit_transactions(transaction_id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: task_billing_settlements task_billing_settlements_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_settlements
    ADD CONSTRAINT task_billing_settlements_task_id_fkey FOREIGN KEY (task_id) REFERENCES task_billing_snapshots(task_id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: task_billing_snapshots task_billing_snapshot_hold_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_snapshots
    ADD CONSTRAINT task_billing_snapshot_hold_fk FOREIGN KEY (hold_transaction_id) REFERENCES credit_transactions(transaction_id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: task_billing_snapshots task_billing_snapshot_price_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_snapshots
    ADD CONSTRAINT task_billing_snapshot_price_fk FOREIGN KEY (sku, price_version_id, price_version, unit_credits) REFERENCES billing_price_versions(sku, price_version_id, version, unit_credits) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: task_billing_snapshots task_billing_snapshots_sku_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_billing_snapshots
    ADD CONSTRAINT task_billing_snapshots_sku_fkey FOREIGN KEY (sku) REFERENCES billing_skus(sku) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: task_submission_claims task_submission_claims_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY task_submission_claims
    ADD CONSTRAINT task_submission_claims_task_id_fkey FOREIGN KEY (task_id) REFERENCES task_billing_snapshots(task_id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
--
CREATE OR REPLACE FUNCTION publish_billing_prices(
  p_sku text,
  p_display_name text,
  p_description text,
  p_prices jsonb,
  p_actor_id bigint
)
RETURNS SETOF billing_price_versions
LANGUAGE plpgsql SECURITY DEFINER
SET search_path TO 'public', 'pg_catalog', 'pg_temp'
AS $fn$
DECLARE
  v_resolution TEXT;
  v_value JSONB;
  v_unit BIGINT;
  v_old_id BIGINT;
  v_old_version BIGINT;
  v_old_unit BIGINT;
  v_version BIGINT;
  v_new_id BIGINT;
  v_materialized BOOLEAN;
  v_inserted BIGINT;
  v_result billing_price_versions%ROWTYPE;
BEGIN
  p_sku := lower(btrim(p_sku));
  p_display_name := btrim(p_display_name);
  p_description := btrim(COALESCE(p_description, ''));
  IF p_sku IS NULL OR p_sku !~ '^[a-z][a-z0-9-]{0,31}:[a-z][a-z0-9._-]{0,94}$'
     OR char_length(p_sku) > 128 OR p_display_name IS NULL
     OR char_length(p_display_name) NOT BETWEEN 1 AND 120
     OR char_length(p_description) > 1000 OR p_prices IS NULL
     OR jsonb_typeof(p_prices) <> 'object' OR p_actor_id IS NULL OR p_actor_id <= 0
     OR NOT (p_prices ? '1k' OR p_prices ? 'default') THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing prices publish command';
  END IF;

  IF EXISTS (
    SELECT 1 FROM jsonb_object_keys(p_prices) AS keys(key)
    WHERE key NOT IN ('default', '1k', '2k', '4k')
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'invalid billing prices resolution';
  END IF;

  INSERT INTO billing_skus (sku, display_name, description, enabled, created_by, updated_by)
  VALUES (p_sku, p_display_name, p_description, TRUE, p_actor_id, p_actor_id)
  ON CONFLICT (sku) DO NOTHING;
  GET DIAGNOSTICS v_inserted = ROW_COUNT;
  v_materialized := v_inserted > 0;

  PERFORM 1 FROM billing_skus WHERE sku = p_sku FOR UPDATE;
  UPDATE billing_skus
  SET display_name = p_display_name, description = p_description,
      enabled = CASE WHEN NOT EXISTS (
        SELECT 1 FROM billing_current_prices AS current_prices
        WHERE current_prices.sku = p_sku
          AND current_prices.output_resolution IN ('default', '1k')
      ) THEN TRUE ELSE enabled END,
      updated_by = p_actor_id, updated_at = clock_timestamp()
  WHERE sku = p_sku;

  FOREACH v_resolution IN ARRAY ARRAY['default', '1k', '2k', '4k'] LOOP
    v_value := p_prices -> v_resolution;
    IF v_value IS NULL THEN
      DELETE FROM billing_current_prices
      WHERE sku = p_sku AND billing_current_prices.output_resolution = v_resolution;
      CONTINUE;
    END IF;
    IF jsonb_typeof(v_value) <> 'number' THEN
      RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'resolution price must be an integer';
    END IF;
    v_unit := (v_value #>> '{}')::BIGINT;
    IF v_unit < 0 OR v_unit > 9000000000000000 OR (v_value #>> '{}') !~ '^[0-9]+$' THEN
      RAISE EXCEPTION USING ERRCODE = 'IB101', MESSAGE = 'resolution price is out of range';
    END IF;

    SELECT current.price_version_id, prices.version, prices.unit_credits
    INTO v_old_id, v_old_version, v_old_unit
    FROM billing_current_prices current
    JOIN billing_price_versions prices
      ON prices.sku = current.sku AND prices.output_resolution = current.output_resolution
     AND prices.price_version_id = current.price_version_id
    WHERE current.sku = p_sku AND current.output_resolution = v_resolution;

    SELECT COALESCE(max(prices.version), 0) + 1 INTO v_version
    FROM billing_price_versions prices
    WHERE prices.sku = p_sku AND prices.output_resolution = v_resolution;
    INSERT INTO billing_price_versions (sku, output_resolution, version, unit_credits, created_by)
    VALUES (p_sku, v_resolution, v_version, v_unit, p_actor_id)
    RETURNING billing_price_versions.* INTO v_result;
    v_new_id := v_result.price_version_id;
    INSERT INTO billing_current_prices (sku, output_resolution, price_version_id)
    VALUES (p_sku, v_resolution, v_new_id)
    ON CONFLICT ON CONSTRAINT billing_current_prices_pkey DO UPDATE
      SET price_version_id = EXCLUDED.price_version_id, updated_at = clock_timestamp();
    IF v_resolution IN ('default', '1k') THEN
      UPDATE billing_skus SET current_price_version_id = v_new_id WHERE sku = p_sku;
    END IF;
    INSERT INTO billing_admin_actions (action_type, sku, price_version_id, actor_id, reason, metadata)
    VALUES ('create_price', p_sku, v_new_id, p_actor_id, 'price published',
      jsonb_build_object('output_resolution', v_resolution, 'materialized', v_materialized,
        'old', CASE WHEN v_old_id IS NULL THEN NULL ELSE jsonb_build_object('price_version_id', v_old_id, 'version', v_old_version, 'unit_credits', v_old_unit) END,
        'new', jsonb_build_object('price_version_id', v_new_id, 'version', v_version, 'unit_credits', v_unit), 'outcome', 'published'));
    RETURN NEXT v_result;
  END LOOP;
END;
$fn$;

REVOKE ALL ON FUNCTION publish_billing_prices(TEXT, TEXT, TEXT, JSONB, BIGINT) FROM PUBLIC;
