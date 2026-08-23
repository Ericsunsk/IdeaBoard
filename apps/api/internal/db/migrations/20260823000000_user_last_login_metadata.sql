ALTER TABLE users
  ADD COLUMN last_login_ip VARCHAR(45),
  ADD COLUMN last_login_country VARCHAR(2),
  ADD CONSTRAINT chk_users_last_login_ip
    CHECK (last_login_ip IS NULL OR last_login_ip <> ''),
  ADD CONSTRAINT chk_users_last_login_country
    CHECK (last_login_country IS NULL OR last_login_country ~ '^[A-Z]{2}$');
