# Production QA 清理边界

Production smoke test 可以复用正式 API、Provider 和 Credits 路径，但测试资源必须带唯一 `QA_TEST_RUN_ID` 标记，例如 `qa-20260831-abc123`。Board name、Library asset title/source ID 至少一处必须包含该 marker。

清理使用 `scripts/qa-production-cleanup.sh`，默认只执行 owner/marker 校验并输出 dry-run 清单：

```bash
QA_API_BASE_URL=<api-origin> \
QA_COOKIE_FILE=/secure/cookies.txt \
QA_OWNER_USER_ID=<usr_...> \
QA_WORKSPACE_ID=<ws_...> \
QA_TEST_RUN_ID=qa-20260831-abc123 \
  scripts/qa-production-cleanup.sh \
  --board-id <qa-board-id> \
  --library-asset-id <qa-library-asset-id>
```

确认清单、session owner 和 marker 均正确后，才追加 `--confirm`。脚本只调用现有 authenticated workspace-scoped v1 API：`POST /api/v1/workspaces/:workspace_id/boards/:board_id/archive` 归档 Board，`DELETE /api/v1/workspaces/:workspace_id/library/assets/:id` 删除 library asset；不会删除 Task、Task Billing snapshot、Credit Ledger、Provider、Prompt、Account 或 User。

脚本不直接删除 Storage object。删除 Board 后，Media refs 由现有 Media reclamation worker（`media_reclamation` River queue）收敛；对象仍遵守 grace period 和 worker claim 保护。当前没有可手动执行的 `media-reclamation scan` 只读命令（未实现）：不要拼装该命令；需要核对未引用对象时以数据库引用反查与各 profile object listing 的集合差异为准（见 [deployment.md](./deployment.md) 的对象 orphan 检查步骤），差异对象先标记、观察一个 `GracePeriod`，禁止直接批量删除。

`Task` 与 Credits 记录是 immutable audit facts，即使 Ideas 免费也必须保留；失败任务不应通过清理伪造退款或删除账务事实。

