# Workspace 新表 Runtime Grants 补授手册（#830）

> 状态（2026-09-28）：RESOLVED。durable fix 已合入：`apps/api/internal/platform/database/runtime_privileges.go`
> 的 policy 已覆盖 workspace 表（含序列），`database_wiring_db_test.go`
>（`workspaceRuntimeTableNames` / `workspaceRuntimeSequenceNames`）在启动测试中断言覆盖。
> 本手册保留为 audit 证据与应急补授 SQL；新建表必须同步进 policy + 启动断言，否则 §2 所述静默遗漏会重演。

> 历史状态：AUDIT ONLY。`finalize-migrate` 的 7 个 privilege policy 未覆盖 16 张 #809 workspace 新表，
> runtime role（`DB_USER`，如 dev 的 `ideaboard_api`）对这些表零权限，首次读写即 `permission denied`。
> 本手册给出 ops 可直接执行的补授 SQL（text，未执行）与验证方法。

## 1. Grant 矩阵（runtime role 实测结论）

`OK` = policy 已覆盖；`MISSING` = 无任何 policy 覆盖，`has_table_privilege` 全为 false
（在 `ideaboard-test-pg:5433` 隔离 scratch DB 实测，`Configure` 等效 grants 下复现）。

| 表 | SELECT | INSERT | UPDATE | DELETE | 身份序列 USAGE | 结论 |
| --- | --- | --- | --- | --- | --- | --- |
| tasks | ✅ | ✅ | ✅ | ✅ | ✅ | OK（对照组） |
| task_outputs | ✅ | ✅ | ✅ | ✅ | ✅ | OK（对照组） |
| library_folders / library_assets | ✅ | ✅ | ✅ | ✅ | ✅ | OK（对照组） |
| media_assets | ✅ | ✅ | ✅ | ✅ | ✅ | OK（对照组） |
| organizations | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING（需 S,I,U） |
| organization_memberships | ❌ | ❌ | ❌ | ❌ | —（无身份列） | MISSING（需 S,I,U,D） |
| workspaces | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING（需 S,I,U） |
| workspace_memberships | ❌ | ❌ | ❌ | ❌ | —（无身份列） | MISSING（需 S,I,U,D） |
| billing_accounts | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING（需 S,I） |
| upload_sessions | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING（需 S,I,U） |
| media_asset_derivatives | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING（需 S,I,U upsert） |
| library_materializations | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING（materialization 为 internal-only，无 HTTP 运行时 DML，latent） |
| workspace_events / event_subscriptions / event_deliveries | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING（暂无 runtime 引用，latent） |
| board_documents / board_revisions | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING（Phase 4 follow-up，未接线，latent） |
| task_billing_snapshots | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING（需 S,I；UPDATE/DELETE 由 trigger 挡） |
| task_billing_settlements | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING（需 I；release 的 UPDATE 走 SECURITY DEFINER 函数，见 §4） |
| control_audit_records | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING（需 I，write-only sink） |

所需动词依据：`apps/api/internal/db/queries/workspaces.sql`、`organizations.sql`、
`workspace_tasks.sql`、`workspace_media.sql`、`control_audit.sql` 的实际语句，
以及 `tasking/application/billing/service.go:206,217`、
`tasking/application/billing/usage.go:49`、`platform/audit/audit.go:102` 的直接调用。

## 2. 根因（证据链）

1. Migration 本身零 GRANT：`apps/api/internal/db/migrations/20260924000001_baseline.sql`、
   `apps/api/internal/db/schema/fragments/`、`schema.sql`、`apps/api/atlas.hcl` 中无任何 `GRANT`。
2. 唯一授权入口是 `finalize-migrate` → `finalizeAppDatabases`
  （`apps/api/internal/runtime/composition/database_wiring.go:126`），用
   `runtimePrivilegePolicies()`（同文件 :136-146）的 7 个 policy 做
   `dbprivileges.Configure`；`Configure` 只对 policy 列出的对象执行 GRANT
   （`apps/api/internal/platform/database/privileges/sql.go:47-51`），未列出即默认零权限。
3. 程序化核对 7 个 policy 的全部表名：16 张表无一被覆盖
   （API policy 见 `apps/api/internal/platform/database/runtime_privileges.go:5-58`；
   其余 6 个为 credits / billing / river / rewards / payments / searchcatalog，
   均为各自前缀域，无 workspace 表）。
4. 两个遮蔽效应导致存量测试全绿：
   - 绝大多数 domain 测试以 owner 身份直连（`testdb` 超级用户 + 隔离 schema），从不经过 runtime role；
   - 唯一覆盖 runtime role 的启动测试
     （`database_wiring_db_test.go:48-77`）只断言 `provider_endpoints` 与
     `library_asset_media_assets`；startup `Verify` 也只校验已列出对象
     （`privileges/sql.go` verify queries），缺表不会 fail closed，
     故障只在业务请求时以 500/`permission denied` 暴露。

## 3. 补授 SQL（ops 执行，单事务）

`<RUNTIME_ROLE>` 替换为实际 `DB_USER`（dev 为 `ideaboard_api`）。
序列名遵循 `GENERATED ALWAYS AS IDENTITY` 默认命名（`<表>_internal_id_seq`，
与 `apiOwnedSequenceNames` 中 `tasks_internal_id_seq` 等既有命名一致）；
`organization_memberships` / `workspace_memberships` 无身份列，无需序列授权。
风格与既有 API policy 对齐：表统一 SIUD（`task_billing_*` 的不可变性由 trigger 保证，
见 `billing_test.go:266-276` 的 tamper 否定用例），序列统一 `USAGE, SELECT, UPDATE`。

```sql
BEGIN;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE
  public.organizations, public.organization_memberships,
  public.workspaces, public.workspace_memberships,
  public.billing_accounts,
  public.upload_sessions, public.media_asset_derivatives,
  public.library_materializations,
  public.workspace_events, public.event_subscriptions, public.event_deliveries,
  public.event_delivery_attempts,
  public.board_documents, public.board_revisions,
  public.task_billing_snapshots, public.task_billing_settlements
TO "<RUNTIME_ROLE>";
GRANT SELECT, INSERT ON TABLE public.control_audit_records TO "<RUNTIME_ROLE>";
REVOKE UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER ON TABLE public.control_audit_records FROM "<RUNTIME_ROLE>";
GRANT USAGE, SELECT, UPDATE ON SEQUENCE
  public.organizations_internal_id_seq,
  public.workspaces_internal_id_seq,
  public.billing_accounts_internal_id_seq,
  public.upload_sessions_internal_id_seq,
  public.media_asset_derivatives_internal_id_seq,
  public.library_materializations_internal_id_seq,
  public.workspace_events_internal_id_seq,
  public.event_subscriptions_internal_id_seq,
  public.event_deliveries_internal_id_seq,
  public.event_delivery_attempts_internal_id_seq,
  public.board_revisions_internal_id_seq,
  public.control_audit_records_internal_id_seq
TO "<RUNTIME_ROLE>";
COMMIT;
```

> [!NOTE]
> `organization_memberships` / `workspace_memberships` / `board_documents` /
> `task_billing_snapshots` / `task_billing_settlements` 无身份列，故无序列授权
> （之前版本误列了并不存在的 `*_internal_id_seq`，已修正）。
> `REVOKE` 方向无需处理：`Configure` 只对已列出对象做 `REVOKE ... FROM PUBLIC` /
> forbidden-privilege 回收，从不触碰未列出表；手工 GRANT 在后续 `finalize-migrate`
> 重跑后依然保留。但这只是止血—— durable fix 已落地
> （`runtime_privileges.go` 的 policy + 启动断言），合并后重跑一次
> `finalize-migrate` 使 policy-as-code 与实际一致。

## 4. 验证 SQL（补授前后均可跑）

```sql
-- 期望：补授前 17 张表全 f；补授后全 t（control_audit_records 仅 S,I 为 t）
SELECT t AS table_name,
  has_table_privilege('<RUNTIME_ROLE>', t, 'SELECT,INSERT,UPDATE,DELETE') AS siud,
  has_sequence_privilege('<RUNTIME_ROLE>', t || '_internal_id_seq', 'USAGE') AS seq_usage
FROM unnest(ARRAY[
  'organizations','workspaces','billing_accounts','upload_sessions',
  'media_asset_derivatives','library_materializations','workspace_events',
  'event_subscriptions','event_deliveries','event_delivery_attempts',
  'board_revisions','control_audit_records'
]) AS t ORDER BY 1;
-- 无身份列的表（无序列）+ control 只读断言，单独查：
SELECT has_table_privilege('<RUNTIME_ROLE>', 'workspace_memberships', 'SELECT,INSERT,UPDATE,DELETE') AS ws_members_siud,
       has_table_privilege('<RUNTIME_ROLE>', 'organization_memberships', 'SELECT,INSERT,UPDATE,DELETE') AS org_members_siud,
       has_table_privilege('<RUNTIME_ROLE>', 'board_documents', 'SELECT,INSERT,UPDATE,DELETE') AS board_docs_siud,
       has_table_privilege('<RUNTIME_ROLE>', 'task_billing_snapshots', 'SELECT,INSERT') AS snapshots_si,
       has_table_privilege('<RUNTIME_ROLE>', 'task_billing_settlements', 'SELECT,INSERT') AS settlements_si,
       has_table_privilege('<RUNTIME_ROLE>', 'control_audit_records', 'SELECT,INSERT') AS audit_si,
       NOT has_table_privilege('<RUNTIME_ROLE>', 'control_audit_records', 'UPDATE') AS audit_no_update,
       NOT has_table_privilege('<RUNTIME_ROLE>', 'control_audit_records', 'DELETE') AS audit_no_delete;
-- 负例冒烟（补授前应报 permission denied，补授后成功；写后回滚）：
BEGIN;
SET LOCAL ROLE "<RUNTIME_ROLE>";
INSERT INTO workspaces (public_id, name) VALUES ('ws_grant_smoke', 'smoke') RETURNING internal_id;
ROLLBACK;
```

## 5. 回滚

```sql
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE
  public.organizations, public.organization_memberships,
  public.workspaces, public.workspace_memberships,
  public.billing_accounts, public.upload_sessions, public.media_asset_derivatives,
  public.library_materializations, public.workspace_events,
  public.event_subscriptions, public.event_deliveries,
  public.board_documents, public.board_revisions,
  public.task_billing_snapshots, public.task_billing_settlements,
  public.control_audit_records
FROM "<RUNTIME_ROLE>";
-- 序列 REVOKE 同理（USAGE, SELECT, UPDATE），略。
```

## 6. 残留风险（需 owner 跟进，非 ops 动作）

1. `task_billing_settlements` 的 release 路径：runtime 非测试代码中只有 INSERT
   （`service.go:217`），状态流转疑似走 `SECURITY DEFINER` credit 函数；若某条 release
   路径实际以 runtime 身份直接 UPDATE settlements，补授 SIUD 已覆盖，但建议实现方确认。
2. `board_revisions` / `board_documents`、`library_materializations`（HTTP materialization surface 已 owner-approved 删除，internal-only job 记录仍在）、
   `workspace_events*` 当前无 HTTP 运行时查询（latent 缺口）：本次一并授权可避免接线时再踩；
   若坚持最小权限，可先只授 §1 标“需”的 10 张表 + 2 memberships，latent 表在接线变更中补 policy。
3. 建议在 `database_wiring_db_test.go` 的 `assert*Access` 模式上追加 workspace 表断言，
   否则下次加表仍会静默遗漏（本次 audit 的对照实验即用此模式复现）。
