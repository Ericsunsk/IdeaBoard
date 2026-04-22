-- 创建数据库（如果还没有创建）
CREATE DATABASE IF NOT EXISTS ideaboard CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE ideaboard;

-- 用户表（精简版：只保留必要的字段）
DROP TABLE IF EXISTS `users`;
CREATE TABLE `users` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `username` varchar(50) NOT NULL COMMENT '账号（唯一）',
  `password` varchar(255) NOT NULL COMMENT '密码（加密存储）',
  `status` tinyint DEFAULT 1 COMMENT '状态：0-封禁 1-正常',
  `role` varchar(20) DEFAULT 'user' COMMENT '角色：admin-管理员 user-普通用户',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户表';

-- 管理员账号必须在后端启动后通过 init-admin 显式创建：
-- cd apps/api && INIT_ADMIN_PASSWORD='your-strong-password' go run ./cmd init-admin
