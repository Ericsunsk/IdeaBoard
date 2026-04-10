-- 创建管理员账号
-- 密码: admin123 (bcrypt加密)
INSERT INTO users (username, password, status, role, created_at, updated_at) 
VALUES ('admin', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZRGdjGj/n3.rsS6KH6w.H2V7xqOXO', 1, 'admin', NOW(), NOW())
ON DUPLICATE KEY UPDATE updated_at = NOW();
