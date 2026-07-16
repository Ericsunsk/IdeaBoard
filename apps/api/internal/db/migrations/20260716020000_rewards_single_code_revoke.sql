-- migrate:up
ALTER TABLE reward_admin_actions
  DROP CONSTRAINT ck_reward_admin_actions_type,
  ADD CONSTRAINT ck_reward_admin_actions_type CHECK (
    action_type IN (
      'create_campaign', 'update_campaign', 'issue_public_code', 'issue_code_batch',
      'activate_campaign', 'pause_campaign', 'resume_campaign', 'cancel_campaign',
      'delete_campaign', 'revoke_code_batch', 'revoke_code'
    )
  );

-- migrate:down
-- statement:begin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM reward_admin_actions WHERE action_type = 'revoke_code') THEN
    RAISE EXCEPTION 'cannot roll back Rewards single-Code revoke after journal facts exist';
  END IF;
END;
$$;
-- statement:end

ALTER TABLE reward_admin_actions
  DROP CONSTRAINT ck_reward_admin_actions_type,
  ADD CONSTRAINT ck_reward_admin_actions_type CHECK (
    action_type IN (
      'create_campaign', 'update_campaign', 'issue_public_code', 'issue_code_batch',
      'activate_campaign', 'pause_campaign', 'resume_campaign', 'cancel_campaign',
      'delete_campaign', 'revoke_code_batch'
    )
  );
