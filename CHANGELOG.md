# Changelog

本文件记录 IdeaBoard 面向用户和运维的重要变化。纯内部重构、格式调整和生成文件更新不逐条列出。

## Unreleased

### Changed

- 数据库改为 domain-first schema fragments，并由 manifest 生成 canonical `schema.sql`。
- migration history 改为 append-only SHA-256 checksum，并以 `20260814000000_baseline.sql` 作为唯一 clean-break baseline。
- 旧数据库不能通过已删除的历史 migrations 自动升级；部署前必须执行 Fresh Database reset 或从兼容备份恢复。

## v0.1.3 - 2026-08-08

### Added

- 增加 Google OAuth、Email OTP、Password reset、Email change 和 typed transactional email templates。
- 增加 Stripe Idea 充值、退款、Dispute 回收、订单管理和对账能力。
- 增加 Credits Wallet、Reward Code、Usage analytics 与相关 Admin operations。
- 增加基于 signed release manifest、Cosign、GHCR digest 和 restricted SSH 的 Production 自动部署。

### Changed

- Auth 按 Admission、Identity、Social Identity、Transactional Mail 和 runtime lifecycle 拆分边界。
- Canvas、Account、Admin 与支付界面统一交互和视觉规范。
- CI 统一为按变更分类的 required `CI Gate`，并迁移到 Ubicloud managed runners。
- Production stack 统一使用标准 Compose naming、Nginx Proxy Manager override 和 release state。

### Fixed

- 修复 Stripe Checkout tax location、idempotency、重复充值与 checkout shutdown 生命周期问题。
- 修复 Auth/API runtime database privileges、fresh migrations 和 PostgreSQL routine security contracts。
- 修复 Google OAuth 可用性、登录反馈、公开注册联动和 public identity schema 问题。
- 修复 Canvas floating controls 在缩放时的尺寸、定位和输入布局问题。
