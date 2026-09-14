# ADR-0007: Media Delivery Pull Model and Explicit Asset Health

## Context

2026-09 实测暴露了上传→asset→衍生链路的结构性脆弱（起因：外部拖入截图）：

- 上传返回 200 只代表“行写进去了”，不代表 asset 可用。checksum 去重曾长期返回一个原文件已删、衍生终态失败的死 asset，任何重传都 200 命中它，前端节点永久卡在 Preparing（已用窄修复止血：去重命中探活，缺失则退役死行后走新建）。
- 衍生是 push 式异步预计算（River + outbox + 轮询）：失败要靠覆盖式 error 字段取证，board 删除会留下孤儿 asset 行无限重试，前端对终态失败无错误输出。
- 职责重复：服务端预切 thumb/preview 的同时，前端 `useDisplayImagePreview` 也在客户端降采样。

根本问题：链路按“上游永远健康”假设写成，单点坏死会全链卡死且无法自愈。

## Decision

已落地（2026-09-14）：

1. **Tier 只保留为契约名词**（`thumb/preview/large-preview/processing-input` 的语义和调用方不变），物化从 push 预计算改为 **pull 按需渲染**：衍生 `content` 端点不再读盘回源，而是校验归属后 302 到签名后的 imgproxy tier URL，由 imgproxy 首次请求同步渲染 + 自缓存。失败变同步 HTTP 错误，**轮询、outbox、River worker、重试、stuck 四件套已整体删除**（`media_derivative_outbox` 消费链路、`infrastructure/worker` 包、`EnsureDerivative` / `RequestDerivativeRepair` 一并退场）。
2. **`processing-input` 是唯一物化的衍生**：它在 ingestion `Complete` 内同步落行（provider-neutral 契约，task manifest 靠它取输入），不再走异步生成。物化失败会把已提交的 asset 转为 `failed`，不会返回一个 ready-but-unusable 的 asset；仅对历史 task output 保留 manifest-time lazy healing。前端 `resolveMediaAssetProcessingURL` 与 `ensure_tier` 参数同步退化为**单次 metadata 读**。
3. **Asset 健康显性化**：上传去重只复用完全健康的 asset（对象存在 + 关键衍生可用）；终态失败必须能被前端渲染为错误态（重传引导），而不是空白 Preparing。
4. **性能验证后再决定 preview 层的去留**：若实测客户端降采样顶得住列表/画布首屏，则服务端只留原图 + processing-input；顶不住则保留 pull 模式的服务端切图。
5. **Renderer source 必须显式可用**：`builtin-local` 通过 compose 中的 imgproxy sidecar 共享只读 uploads volume；其他 storage profile 不把原图伪装成 tier。`processing-input` 只从已验证的物化 derivative 读取，display tier 在尚未接入 provider source adapter 时返回明确错误。

## Consequences

- 正面：消灭整类“静默卡死”；去重、删除、衍生三处的隐式耦合收敛到一次同步检查；取证不再依赖被覆盖的 error 字段。
- 代价：首次 GET 延迟上升（需要渲染超时 + 降级策略）；非 local profile 的 display tier 需要后续 provider source adapter 才能启用；S3 出站与客户端算力开销需重新评估；board 删除与 asset 生命周期的归属仍需单独决策（孤儿行问题本 ADR 未覆盖）。
- 排除项：本 ADR 不改变 asset 跨 board 复用的用户级语义，不动计费/任务侧的媒体引用契约。

## Status

Accepted（2026-09-14）。第 1、2 项已实施：后端 `internal/media/imgproxy` + `composition/imgproxy.go` 提供签名 URL 与 302 网关，衍生 worker/outbox 链路已删除；前端 `mediaAssetDelivery` 的 `ensure_tier` 参数与轮询已删除。第 3 项（去重探活）已单独落地。第 4 项待性能实测。

## Vendor Note (2026-09-14)

imgproxy vs Imagor 头对头结论：**维持 imgproxy，不换**。核对过双方一手文档：

- 缓存：调研曾称 imgproxy S3 结果缓存是 Pro 能力，经核官方文档有误——OSS 即支持 `IMGPROXY_CACHE_USE=s3` + 自定义 endpoint（点名 MinIO / R2 / DO Spaces），Imagor 在这点上没有实质优势。
- R2：双方都是 S3 兼容 endpoint 指过去；Imagor 只是文档里点名了 R2，imgproxy 需实测一次，无结构差异。
- 安全：双方都有 HMAC 签名 + 来源白名单 + image bomb 防护；Imagor 默认 SHA1，需显式切 sha256，imgproxy 默认即 SHA256。
- 形态：Imagor 可作 Go library 嵌入，但我们要的是 sidecar，这条反而是 imgproxy 的简单性占优。
- 社区与维护：imgproxy 11k star、双周发版、有商业公司背书；Imagor 4k、单维护者。libvips 同底，性能不会有量级差（Imagor 自跑 benchmark 也是同一量级，只能这么读）。
- 换 Imagor 的触发条件只剩：imgproxy 出现 license 变更、R2 实测不兼容，或需要 Result Storage 打标签做生命周期联动（Imagor 的 tagging + expiration 是它唯一独占的实用特性，但我们的生命周期由 DB 的 reclaim 拥有，价值有限）。
