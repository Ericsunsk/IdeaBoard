-- destructive: approved
ALTER TABLE api_accounts
  DROP CONSTRAINT chk_api_accounts_timeout;

ALTER TABLE api_accounts
  ADD CONSTRAINT chk_api_accounts_timeout
  CHECK (timeout BETWEEN 1 AND 900);
