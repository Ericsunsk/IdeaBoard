# Personal / Organization 资金池切换与验收（#949、#956）

目标合同见 [ADR-0038](../adr/0038-personal-organization-billing-model.md)。本次按
[DEVELOPMENT.md](../../DEVELOPMENT.md#generated-code) 的 pre-launch clean-break 规则，
在 Fresh Database 上应用当前完整 Atlas migration chain；不执行旧多账户拓扑的原地升级。
应用、schema、权限和生成客户端必须来自同一个 release。

本 runbook 取代旧版中“冻结旧账户后改派 Workspace、平移 quota、删除旧账户”的操作步骤。
当前实现没有这些数据转换程序，也不应把隔离库测试通过记为存量资金迁移完成。

## 1. 旧数据处置边界（#949）

旧模型的盘点与处置分类保留如下。表中“保留”指原库 backup 与只读审计材料；
不是在新库同时保留可消费的旧账户，也不是导入或清库授权。

| 旧账户/事实 | 识别范围 | 本次处置 |
| --- | --- | --- |
| 个人默认池 | Default personal Workspace 锚定账户、Owner、Wallet 与 Ledger | 新库按用户建立唯一 Personal 池；旧库资金事实整体保留，不自动导入余额 |
| 同 Owner 的额外独立池 | 非 Default Workspace 锚定账户及其成员、余额、额度 | 不改派、不合并、不清零；非零余额与额度冲突须另行批准处置 |
| 显式 Workspace link | 单用户共享、跨用户共享、跨组织或悬空 link | 新应用不读取/写入 link；旧归属逐项归档，不自动确权或迁入新池 |
| Organization 池 | 组织账户、Owner/Admin/Member、余额与组织 Workspace | 新库每组织共享一池；Personal 资金不注入组织，组织资金不转为 Personal |
| 孤儿保留账户 | 已删 scope 的 Wallet、Ledger、订单、快照与操作记录 | 只读保留；非零余额或未完义务阻止直接替换原库 |
| 额度与预算 | Member Quota、Default Member Quota、Workspace Budget | 新库仅保留账户级 Member Quota 产品规则；不平移旧限额，默认不限额，Organization Owner 永远不限额 |
| 交接邀请 | pending、终态与过期邀请及其审计字段 | 旧记录保留；未完邀请先明确取消、完成或到期，不在新账户上重放 |
| 在途资金义务 | Hold、支付/退款/争议、Task/Vision 及行内 payer 快照 | 始终在原库按原 payer 收敛；不复制任务到新库重放，不改写 Ledger/Postings 或历史 payer |

此前开发库盘点是测试数据示例，不能替代切换当日的实际盘点。
若要保留任一存量资金账户，必须先另行制定并批准确定性导入方案、逐账户对账及恢复演练；
本次 Fresh 验收不包含该能力。对非零额外池、跨用户 link、非零孤儿余额、冲突归属、
quota 冲突、未完交接邀请、关闭组织余额的处置，均不能从“无需兼容旧实现”推定获得批准。

## 2. 切换前置条件

1. 确认目标是隔离 Fresh Database。若将替换现有环境，先按
   [data-protection.md](./data-protection.md) 完成 PostgreSQL、Auth 与 uploads 的配套 backup，
   验证 checksum 和恢复能力，记录 release digest、Atlas revisions 与数据保留决定。
2. 维护模式阻止新充值、提交、兑换与 Vision 请求。让已接受的任务及支付义务在原库收敛；
   核对 River、对账任务、Hold、支付/退款/争议及 Task/Vision 状态后，再停 API/worker。
   不得为了排空而直接删任务、释放 Hold 或手改 Ledger。
3. 导出原库逐账户可用/预留余额、Ledger 汇总、Hold、订单、Task/Vision payer 快照和未完交接清单。
   任何非零资金或未完义务都必须有独立批准的保留/处置记录；否则保留原环境并停止切换。
4. 不执行本 runbook 以外的搬余额 SQL，不修改 `atlas_schema_revisions`，不添加兼容 migration。
   `ATLAS_CLEAN_BREAK_ALLOW_RESET=true` 仍是一次性破坏性清库授权开关；操作条件及恢复流程见
   [deployment.md](./deployment.md#api-database-baseline)，本 runbook 不执行清库或部署。

## 3. Fresh 初始化与拓扑核验

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

-- 已退役的 link 无新增数据；此表仍在 schema 中，应用不再使用它。
SELECT workspace_id FROM workspace_billing_links;

-- 预算与默认额度表已删除。
SELECT relname FROM pg_class
WHERE relnamespace = 'public'::regnamespace
  AND relname IN ('workspace_budgets', 'billing_account_default_member_quotas');
```

## 4. 集成验收（#956）

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
  这些是新模型正确性测试，不是旧库迁移对账。
- 关闭与恢复：关闭组织立即禁止访问和新消费，在途仍按原 payer 收敛；恢复保留原成员与授权。
  当前关闭实现仅记录 `retained` 处置，组织余额留在组织账户；充值条款未提供前不自动退款，
  不宣称原渠道退款已经实现。
- 删除面：独立 Workspace 账户选择、link 端点、创建后换 payer、账户间资金移动、Workspace Budget、
  Default Member Quota 及邀请前置门均无可用应用路径。底层仍保留的表以 schema 为准。
- 检查面：API 全量测试/构建、Auth build/权限测试、Web 测试/构建/治理、sqlc/Orval 生成一致性、
  schema-release、Atlas integrity/Fresh apply/重复 apply。记录实际命令与结果，未运行项不能记为通过。

关闭 #949/#956 时应分别注明 Fresh-only 的处置范围与实际验证结果；生产停写、实际账户盘点、
backup/restore、真实支付条款及任何存量资金迁入仍是切换前的独立门禁，不由关票代替。

## 5. 失败恢复

初始化或检查失败时保持维护状态并停止发布，不反向修改资金事实。
按照 [deployment.md](./deployment.md) 与 [data-protection.md](./data-protection.md)
恢复配套数据库、uploads 和对应镜像。仅回退应用镜像不能撤销 schema 变化；
不要把新代码与旧数据库混用，也不要为单 Workspace 误删恢复整库。
