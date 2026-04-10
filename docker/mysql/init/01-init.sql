-- 创建数据库（如果还没有创建）
CREATE DATABASE IF NOT EXISTS feng CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE feng;

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

-- 创建默认管理员账号 admin/123456
INSERT INTO `users` (`username`, `password`, `status`, `role`) VALUES
('admin', '$2a$10$N.zmdr9k7uOCQb376NoUnuTJ8iAt6Z5EHsM8lE9lBOsl7iKTVKIUi', 1, 'admin');
