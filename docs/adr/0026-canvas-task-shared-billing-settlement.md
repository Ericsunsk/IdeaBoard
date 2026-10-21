# ADR-0026: Canvas Task Shared-Billing Settlement

## Context

Issue #917 要求把 #916 已迁的 Task 链路按共享账户模型坐实：正式成员在获授权
Canvas 发起生成时使用该 Workspace 的稳定 Billing Account，此后成功、失败、
取消和重试都结算到提交时的账户。#915 落地了稳定解析（显式 link → 组织池 →
个人账户，调用方 tx-join，fail closed），#916 把 Wallet 归属切到账户契约
（Task 提交事务内解析稳定账户并 Hold，终态读行内账户收敛，snapshot 落
billing_account_id），但仍有缺口：task-quotes 预检不解析账户、不查消费
资格（Viewer 也能读 ready，余额不足与资金限制不区分）；终态与提交在借用
guard 事务时事件可能先于 commit 发布；预算/成员额度（945/946）的事务边界
没有预留位。产品模型见桌面共识《IdeaBoard-团队账单池设计共识》（Billing
Account 持有统一余额，多 Workspace 共享，Workspace 管消费位置，User 记实际
操作人）与 [ADR-0024](./0024-billing-account-resolution.md)、
[ADR-0025](./0025-billing-account-wallet-ownership.md)。

## Decision

1. Quote、readiness 与 submit 使用同一付款账户和同一消费资格：预检解析
   submit 会 Hold 的稳定账户（无 owner-wallet 回退），Board 绑定时执行与
   submit 相同的 BoardPolicy submit 判定（正式成员需 Board 编辑权限，
   Viewer 403，财务管理角色不自动取得内容权限），Wallet 资金门按 submit
   Hold 映射返回 402 不足 vs 403 限制（含 frozen 透传）。快照保持纯价格
   （不带余额），准确余额经 workspace billing 读面刷新（同一稳定账户）。
2. Task 行固化三元组（billing_account_id、workspace_id、submitter_user_id，
   快照同行 billing_account_id）：成员退出、角色变化、Owner 变化、付款
   链接变化与后台重试永不改写；终态 Capture/Release 读行内账户与操作人
   收敛；重试只换执行令牌，不新增 Hold。
3. 同一 PostgreSQL transaction 提交 server-side Quote、Credit Hold、Task
   insert、RoutingSnapshot 固化、billing 快照与 River enqueue；终态翻转与
   Capture/Release（含 remainder）同一 transaction；事件只在 commit 后发布，
   借用 guard 事务时经 post-commit hook 延迟（拥有事务同步 stage，无
   registry 时保持 legacy 同步 stage）。提交事务内先取 Workspace 守卫锁
   再写幂等 claim（claim 的 workspace FK 取 `FOR KEY SHARE`，守卫锁后行
   会与并发提交互锁 40P01；守卫先行是唯一的入口串行点，claim 仍在
   Quote/Hold 之前守住重放）。
4. 预算/成员额度（945/946）只预留事务边界：未来消费限制检查加入该结算
   事务（稳定账户解析之后、Hold 之前，读同一 repeatable-read 快照），本
   票不实现额度计算、不宣称。
5. 首期不做：Workspace 钱包、内部拨款、跨账户余额转移、auto topup、订阅、
   有效期、更换付款账户；不保留 legacy/别名/owner-wallet 回退/兼容路由/
   双结算；不删除历史资金事实；不扩展外部协作者付费能力。

## Consequences

- Canvas readiness 语义变严格：余额不足的预检 402（可引导充值），受限
  403 与 Board 拒绝为 not-ready（无充值 call to action），Viewer 在预检
  即不可读 ready。旧客户端若忽略 board_id，仍受账户/资金门约束，提交
  行为不变。
- 后续票（945/946）在预留边界上构建额度执行；中间步骤不单独发布。

## Status

Accepted
