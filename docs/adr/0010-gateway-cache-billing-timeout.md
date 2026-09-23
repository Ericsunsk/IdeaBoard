# ADR-0010: Gateway Cache, Billing, and Timeout Policy

## Context

流量收敛到网关后，缓存、计费、超时三处语义必须一次定死，否则每个 PR 都会重新争论。约束：价格手工调整、不自动联动；面板分工为自有面板只看营收、CF/OpenRouter 面板只看成本；现状单账号 provider 超时默认 120 秒。

## Decision

1. 网关全局缓存开关保持关闭，一切缓存行为由 per-request header 显式声明。文本请求带自定义 cache key（含 model）加 TTL 1 小时 opt-in；图像请求一律 skip-cache；跨用户串味已接受。
2. Cache hit 照常收全价 Ideas；cache 状态头只做运营观测，不进账单 snapshot。网关 client 发出的 body 必须 deterministic。
3. Billing 只管 `Quote` 与价格快照，`ResolveModel(feature)` 归 runtime；管理页可组合展示，但 `Quote` 不读路由状态，加测试钉死价格不受模型影响。
4. 超时三层嵌套：网关总预算 300 秒（单次 120 秒对齐现状默认），worker 执行超时 360 秒，River 17 分钟封顶不变。
5. 可重试失败有界重投（2026-09-21 修订，原"worker 单发失败即终态"已废止）：网关内部重试（上限 5 次）之后仍失败的 **transient 类别**（429/408/5xx/超时/网络错误，以 `ShouldFailoverProviderError` 为准，4xx 鉴权/配额/输入错误除外），worker 在 `tasks.retry_count < 2` 时走 reset + River backoff 整任务重投（每次新 execution token，最多 3 次网关发送）；超限或确定性失败直接终态。重投不延长单次预算，只买延迟后的整任务重试。
6. `resolved_model` 改记网关回执头（`cf-aig-model`/`cf-aig-provider`，直调记请求模型）。

## Consequences

- 正面：缓存省成本但不扰乱计费心智；超时不断裂，杜绝本地判失败远端照计费。
- 代价：慢任务上限被显式化，超 300 秒的 provider 行为需重新评估；跨用户串味与图片明文过境已接受，不新增暴露面。
- 排除项：按模型成本自动调价、Unified Billing、账单 snapshot 记录缓存命中。

## Status

Accepted（2026-09-16，方案封版确认）；第 4–5 条于 2026-09-21 修订：网关 5 次内部重试耗尽后，transient 失败允许最多 2 次 worker 层重投（整任务、River backoff、新 token），替代原"单发失败即终态"。
