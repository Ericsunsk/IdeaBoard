# Personal / Organization 资金池初始化与验收（#956）

目标合同见 [ADR-0038](../adr/0038-personal-organization-billing-model.md)。
项目尚未上线，没有需要保留或迁移的存量账户与资金。按
[DEVELOPMENT.md](../../DEVELOPMENT.md#generated-code) 的 pre-launch clean-break 规则，
在 Fresh Database 上应用当前完整 Atlas migration chain；应用、schema、权限和生成客户端
必须来自同一个 release。此手册只覆盖初始化和新模型验收，不执行部署或数据库重建。

## 1. Fresh 初始化与拓扑核验

按 deployment 的 API migration → Auth migration → 服务启动顺序初始化。
Atlas 必须从当前完整 chain Fresh apply，并再次 apply 验证幂等；不能只导入 `schema.sql`
替代 migrations、River migration 或 runtime privilege setup。

通过正常账户创建流程准备用户、Personal Default、Personal 协作 Workspace、Organization
及多个组织 Workspace 后，用只读查询核对以下结果（任一返回行即停）：

```sql
-- 已创建的用户各恰一个 Default personal Workspace。
SELECT u.id FROM users u
LEFT JOIN workspaces w ON w.owner_user_id = u.id
  AND w.organization_id IS NULL AND w.is_default
GROUP BY u.id HAVING count(w.internal_id) <> 1;

-- Personal 资金池只锚定 Default；不能有额外 per-workspace 账户。
SELECT ba.internal_id FROM billing_accounts ba
JOIN workspaces w ON w.internal_id = ba.workspace_id
WHERE ba.organization_id IS NULL AND (NOT w.is_default OR w.owner_user_id IS NULL);

-- 每个 Default personal Workspace 恰一个 Personal 账户。
SELECT w.internal_id FROM workspaces w
LEFT JOIN billing_accounts ba ON ba.workspace_id = w.internal_id
WHERE w.organization_id IS NULL AND w.is_default
GROUP BY w.internal_id HAVING count(ba.internal_id) <> 1;

-- 每个已创建的 Organization 恰一个共享账户。
SELECT o.internal_id FROM organizations o
LEFT JOIN billing_accounts ba ON ba.organization_id = o.internal_id
GROUP BY o.internal_id HAVING count(ba.internal_id) <> 1;

-- 预算与默认额度表已删除。
SELECT relname FROM pg_class
WHERE relnamespace = 'public'::regnamespace
  AND relname IN ('workspace_budgets', 'billing_account_default_member_quotas');
```

## 2. 集成验收（#956）

- 账户与 scope：同一用户多个 Personal Workspace 解析同一池；多个组织 Workspace 解析组织池；
  加入组织不改变 Personal payer；删除普通 Workspace 不删除余额或历史资金事实。
- 访问与邀请：Personal Default 拒邀，普通 Personal Workspace 可由 Owner/Admin 邀请；
  组织 Member 必须先加入组织再逐 Workspace 授权；组织 Owner/Admin 自动访问所有组织 Workspace；
  Admin 降级后只保留明确授权；用户最多属于一个组织。
- Quota：验证五种周期、缺席 unlimited、跨 Workspace 汇总、额度与余额耗尽的不同信号；
  Personal 仅账户持有人管理额度；组织 Owner 永远不限额，Admin 不得设置 Owner/Admin 额度；
  权限读取失败必须阻止管理操作。
- 结算：用新库 fixture 验证 Task/Vision Hold→Capture/Release、失败释放、原 payer 快照、
  退款/争议与幂等；按 fixture 的预期金额逐账户核对 available/reserved、Ledger 和快照。
- 永久关闭：仅 Owner 可执行，提交前二次确认必须清楚展示不可恢复、全部组织 Workspace
  无法访问、余额不能再使用且不自动退款。未确认不得发送关闭请求；重复关闭不得重复处理资金。
  在途 Task、支付订单、退款、争议及待处理 Owner 交接阻止关闭，不能绕过事务内检查。
  关闭后组织及其 Workspace 的读写、新消费与充值被禁止；恢复 API/客户端/入口均已删除。
  余额不得转入 Personal，也不得通过重新创建组织、重入或钱包恢复重新获得使用权。
  资金账户、Ledger 与支付历史保持可核对；关闭不记为消费，不删除或改写账务记录。
- 退款边界：购买原则上不退款，法律要求或购买时明确承诺除外；关闭本身不自动退款。
  例外退款和争议仍关联原 Payment Order、原 payer 与原支付渠道，不能记作 Personal 转账。
- 删除面：独立 Workspace 账户选择、link 端点、创建后换 payer、账户间资金移动、Workspace Budget、
  Default Member Quota 及邀请前置门均无可用应用路径。底层仍保留的表以 schema 为准。
- 检查面：API 全量测试/构建、Auth build/权限测试、Web 测试/构建/治理、sqlc/Orval 生成一致性、
  schema-release、Atlas integrity/Fresh apply/重复 apply。记录实际命令与结果，未运行项不能记为通过。

关闭 #956 时注明实际运行的验证及尚未完成项。未上线且无存量，因此不设置旧账户盘点、
余额迁移或生产切换门禁；#955 按永久关闭、余额使用权终止和退款例外的目标合同独立验收。

## 3. 初始化失败处理

初始化或检查失败时停止启动服务，在隔离测试数据库中修复并重新验证当前完整 migration chain。
不要跳过失败 migration、手改 `atlas_schema_revisions` 或混用不匹配的应用与 schema。
本手册不授权删除或重建已有开发数据库；需要重建时先明确目标及数据可丢弃范围，
按 [deployment.md](./deployment.md) 的 clean-break 规则操作。
