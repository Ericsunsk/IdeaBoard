-- #648 Canonical Outpaint Task Contract and Schema v4 Migration
--
-- Breaking change: the legacy `extend` outpaint control is renamed to
-- `outpaint`, and the top-level/nested task schema versions move from 3 to 4.
-- Keep the execution plan object intact: it is a durable contract owned by
-- workflow and has no `kind` field. Only the outpaint runtime discriminator
-- is renamed in the nested plan.

WITH parsed AS (
  SELECT id,
    CASE
      WHEN params IS JSON OBJECT THEN params::jsonb
      ELSE NULL::jsonb
    END AS payload
  FROM tasks
  WHERE params IS NOT NULL
), rewritten AS (
  SELECT
    id,
    (
      CASE
        WHEN payload ? 'extend'
        THEN (payload - 'extend') || CASE
          WHEN payload ? 'outpaint' THEN '{}'::jsonb
          ELSE jsonb_build_object('outpaint', payload -> 'extend')
        END
        ELSE payload
      END
      || CASE
        WHEN (payload ->> 'schema_version') IN ('3', '4')
        THEN jsonb_build_object('schema_version', 4)
        ELSE '{}'::jsonb
      END
      || CASE
        WHEN jsonb_typeof(payload -> 'execution_plan') = 'object'
        THEN jsonb_build_object(
          'execution_plan',
          CASE
            WHEN (payload -> 'execution_plan' ->> 'schema_version') = '3'
            THEN CASE
              WHEN (payload -> 'execution_plan' ->> 'feature') = 'outpaint'
                OR (payload ->> 'feature') = 'outpaint'
                OR (payload -> 'execution_plan' -> 'runtime' ->> 'control_contract') = 'extend'
              THEN jsonb_set(
                jsonb_set(
                  (payload -> 'execution_plan') - 'kind',
                  '{schema_version}',
                  '4'::jsonb,
                  true
                ),
                '{runtime,control_contract}',
                '"outpaint"'::jsonb,
                true
              )
              ELSE jsonb_set(
                (payload -> 'execution_plan') - 'kind',
                '{schema_version}',
                '4'::jsonb,
                true
              )
            END
            WHEN (payload -> 'execution_plan' -> 'runtime' ->> 'control_contract') = 'extend'
              AND (
                (payload -> 'execution_plan' ->> 'feature') = 'outpaint'
                OR (payload ->> 'feature') = 'outpaint'
              )
            THEN jsonb_set(
              (payload -> 'execution_plan') - 'kind',
              '{runtime,control_contract}',
              '"outpaint"'::jsonb,
              true
            )
            ELSE (payload -> 'execution_plan') - 'kind'
          END
        )
        ELSE '{}'::jsonb
      END
    )::text AS params
  FROM parsed
  WHERE
    (payload ->> 'schema_version') IN ('3', '4')
    OR payload ? 'extend'
    OR (payload -> 'execution_plan' ->> 'schema_version') = '3'
    OR (payload -> 'execution_plan' ->> 'kind') = 'canvas-extend'
    OR (payload -> 'execution_plan' -> 'runtime' ->> 'control_contract') = 'extend'
)
UPDATE tasks AS target
SET params = rewritten.params
FROM rewritten
WHERE target.id = rewritten.id;
