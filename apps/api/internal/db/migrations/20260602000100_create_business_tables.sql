-- migrate:up
CREATE TABLE IF NOT EXISTS users (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(50) NOT NULL,
  auth_user_id VARCHAR(128) NULL,
  storage_uid CHAR(6) NOT NULL,
  avatar VARCHAR(1024) NOT NULL DEFAULT '',
  role VARCHAR(20) NOT NULL DEFAULT 'user',
  status TINYINT NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY idx_users_username (username),
  UNIQUE KEY idx_users_auth_user_id (auth_user_id),
  UNIQUE KEY idx_users_storage_uid (storage_uid)
);

CREATE TABLE IF NOT EXISTS users_used_uids (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  storage_uid CHAR(6) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY idx_users_used_uids_storage_uid (storage_uid)
);

CREATE TABLE IF NOT EXISTS api_accounts (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  channel_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  platform VARCHAR(50) NOT NULL,
  api_type VARCHAR(50) NOT NULL,
  model_set VARCHAR(255) NOT NULL DEFAULT '',
  api_key TEXT NOT NULL,
  proxy_url TEXT NOT NULL,
  max_concurrent INT NOT NULL DEFAULT 3,
  timeout INT NOT NULL DEFAULT 120,
  extra_config TEXT NOT NULL,
  status TINYINT NOT NULL DEFAULT 1,
  fail_count INT NOT NULL DEFAULT 0,
  weight INT NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  KEY idx_api_accounts_status (status),
  KEY idx_api_accounts_channel_id (channel_id)
);

CREATE TABLE IF NOT EXISTS tasks (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  task_id VARCHAR(64) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  channel_id BIGINT UNSIGNED NOT NULL,
  account_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  prompt TEXT NOT NULL,
  user_prompt TEXT NOT NULL,
  params MEDIUMTEXT NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  lease_token VARCHAR(128) NOT NULL DEFAULT '',
  retry_count INT NOT NULL DEFAULT 0,
  error_msg TEXT NOT NULL,
  result MEDIUMTEXT NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  completed_at DATETIME(3) NULL,
  UNIQUE KEY idx_tasks_task_id (task_id),
  KEY idx_tasks_user_id (user_id),
  KEY idx_tasks_account_id (account_id),
  KEY idx_tasks_status (status),
  KEY idx_tasks_lease_token (lease_token),
  KEY idx_tasks_created_at (created_at)
);

CREATE TABLE IF NOT EXISTS task_outputs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  task_id VARCHAR(64) NOT NULL,
  role VARCHAR(32) NOT NULL DEFAULT 'image',
  url TEXT NOT NULL,
  storage_key TEXT NOT NULL,
  mime_type VARCHAR(64) NOT NULL DEFAULT '',
  width INT NOT NULL DEFAULT 0,
  height INT NOT NULL DEFAULT 0,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  sort_index INT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  KEY idx_task_outputs_task_sort (task_id, sort_index)
);

CREATE TABLE IF NOT EXISTS task_daily_stats (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  stat_date CHAR(10) NOT NULL,
  model_name VARCHAR(128) NOT NULL DEFAULT '',
  user_key VARCHAR(128) NOT NULL DEFAULT '',
  total BIGINT NOT NULL DEFAULT 0,
  completed BIGINT NOT NULL DEFAULT 0,
  failed BIGINT NOT NULL DEFAULT 0,
  latency_seconds_sum DOUBLE NOT NULL DEFAULT 0,
  latency_count BIGINT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY uniq_task_daily_stat (stat_date, model_name, user_key)
);

CREATE TABLE IF NOT EXISTS task_stat_events (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  task_id VARCHAR(64) NOT NULL,
  stat_date CHAR(10) NOT NULL,
  model_name VARCHAR(128) NOT NULL DEFAULT '',
  user_key VARCHAR(128) NOT NULL DEFAULT '',
  status VARCHAR(20) NOT NULL,
  latency_seconds DOUBLE NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY uniq_task_stat_event_task_id (task_id),
  KEY idx_task_stat_events_stat_date (stat_date)
);

CREATE TABLE IF NOT EXISTS task_daily_stat_backfills (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  stat_date CHAR(10) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY uniq_task_daily_stat_backfill_date (stat_date)
);

CREATE TABLE IF NOT EXISTS channels (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  type VARCHAR(50) NOT NULL DEFAULT 'openai-compatible',
  name VARCHAR(100) NOT NULL,
  api_url VARCHAR(500) NOT NULL,
  status TINYINT NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
);

CREATE TABLE IF NOT EXISTS system_configs (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `key` VARCHAR(100) NOT NULL,
  value TEXT NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY idx_system_configs_key (`key`)
);

CREATE TABLE IF NOT EXISTS line_draft_modes (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  cover_image TEXT NOT NULL,
  prompt TEXT NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  is_enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  KEY idx_line_draft_modes_sort (sort_order, id),
  KEY idx_line_draft_modes_enabled_sort (is_enabled, sort_order, id)
);

CREATE TABLE IF NOT EXISTS user_boards (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  board_id VARCHAR(100) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(60) NOT NULL,
  state LONGTEXT NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_user_boards_board_id (board_id),
  UNIQUE KEY idx_user_board_name (user_id, name),
  KEY idx_user_updated (user_id, updated_at)
);

CREATE TABLE IF NOT EXISTS board_members (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  board_id VARCHAR(100) NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  role VARCHAR(20) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY idx_board_member (board_id, user_id),
  KEY idx_board_member_user (user_id, board_id)
);

CREATE TABLE IF NOT EXISTS board_share_tokens (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  board_id VARCHAR(100) NOT NULL,
  token VARCHAR(128) NOT NULL,
  role VARCHAR(20) NOT NULL,
  created_by BIGINT UNSIGNED NOT NULL,
  revoked TINYINT(1) NOT NULL DEFAULT 0,
  expires_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY idx_board_share_tokens_token (token),
  KEY idx_board_share_token_board (board_id, created_by, revoked, expires_at)
);

-- migrate:down
DROP TABLE IF EXISTS board_share_tokens;
DROP TABLE IF EXISTS board_members;
DROP TABLE IF EXISTS user_boards;
DROP TABLE IF EXISTS line_draft_modes;
DROP TABLE IF EXISTS system_configs;
DROP TABLE IF EXISTS channels;
DROP TABLE IF EXISTS task_daily_stat_backfills;
DROP TABLE IF EXISTS task_stat_events;
DROP TABLE IF EXISTS task_daily_stats;
DROP TABLE IF EXISTS task_outputs;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS api_accounts;
DROP TABLE IF EXISTS users_used_uids;
DROP TABLE IF EXISTS users;
