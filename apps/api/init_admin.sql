-- Deprecated: do not seed administrator accounts from SQL.
-- Use the runtime flow instead:
--   INIT_ADMIN_PASSWORD='your-strong-password' go run ./cmd init-admin
SELECT 'init-admin-required' AS notice;
