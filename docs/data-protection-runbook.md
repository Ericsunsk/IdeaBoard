# IdeaBoard 数据保护运维手册

本文档聚焦 PostgreSQL backup、restore 与密钥材料处理。账号安全操作边界见 [AUTH_OPERATION_RUNBOOK.md](./AUTH_OPERATION_RUNBOOK.md)，部署步骤见 [DEPLOYMENT.md](../DEPLOYMENT.md)。

## PostgreSQL Backup 安全基线

`ideaboard update` 生成的 `postgres-*.sql` 会包含完整业务库数据，其中包括：

- `api_account_secrets`：上游模型/API 渠道的 API key、token 或外部 secret 引用。
- `storage_profile_secrets`：S3-compatible 存储的 access key、secret key 等对象存储凭据。
- Better Auth 的 `user`、`session`、`account`、`verification` 等身份与会话相关表。
- 业务用户、任务、画板、媒体索引与系统配置。

因此 PostgreSQL backup 必须按 secret artifact 处理，而不是普通日志或普通导出文件。

## 处理要求

- 建议对 backup 加密落盘，并把密钥交给既有 secret 管理流程。
- 限制下载、传输、复制与共享范围，只允许最小必要运维人员访问。
- 不要把 `postgres-*.sql` 上传到工单、聊天工具、未加密对象存储或个人网盘。
- 排障完成后清理临时副本、本地解压文件与一次性传输位置。
- 记录 restore 操作人、时间、来源 backup、目标环境与后续 rotation 结果。

## Restore 后 Rotation

完成 restore 后，按受影响范围轮换：

1. 上游模型/API 渠道密钥，对应 `api_account_secrets`。
2. S3-compatible 对象存储 access key/secret，对应 `storage_profile_secrets`。
3. 生产环境 Better Auth secret 与 auth internal key，只在怀疑 backup 泄露或传输边界失控时轮换。

轮换后执行 smoke 检查，确认登录、任务提交、结果回写、上传与 `/uploads/*` 访问正常。
