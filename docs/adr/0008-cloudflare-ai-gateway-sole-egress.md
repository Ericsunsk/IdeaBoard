# ADR-0008: Cloudflare AI Gateway as Sole AI Egress

## Context

AI 调用链路散落在各供应商 SDK 里：worker 内每个供应商一套 adapter（请求 shaping、传输、错误收敛、重试），AccountPool 持有账号 key、失败计数与调度。新供应商等于新代码，成本线性增长。同时自建运维 metrics 与 CF/OpenRouter 面板视角重叠。参见 [DESIGN.md](../../DESIGN.md) 系统形态与 worker 契约。

## Decision

1. Cloudflare AI Gateway 是唯一 AI 出口；OpenRouter 只是其后的一个 provider，不存在双出口。鉴权模式为 BYOK，key 存 CF。
2. 端点选型只有 provider-native 一种：所有调用带显式 provider + path 直透，不存在 OpenAI-compatible 改写面；chat 判定（`IsChat`）只驱动方言 shaping，不选端点。
3. 统一 schema 只覆盖文本；图像保持 provider 原生格式透传，方言层保留为无状态纯函数。
4. Fallback 拓扑放 dynamic route 版本化管理，改拓扑不碰代码；`MaxFailCount = 5` 对齐为总尝试 5 次，同模型换 key 优先。
5. 探测走主网关直调 provider（绕开 fallback 拓扑），`cf-aig-max-attempts: 1` 单发，结果只写日志；实施时实测单发语义。
6. 每个网关请求附 task id 做 metadata；网关 ID 按环境隔离；token 取最小权限并配 budget cap。

## Consequences

- 正面：新供应商零代码（endpoint＋现存方言都支持才成立）；重试/熔断集中管理，Gateway transport 统一禁用缓存；排障有 task id 串联。
- 代价：接受 CF 单点；网关内部语义（重试上限 5 次、exact-match 缓存、payload 默认存储）成为外部契约，pin 文档版本跟进。
- 后续：Guardrails 接入另起 ticket；Cloudflare Images 替代 imgproxy 仅占位。

## Status

Accepted（2026-09-16，方案封版确认）。
