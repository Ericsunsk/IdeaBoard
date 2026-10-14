# ADR-0007: Media Delivery Pull Model and Explicit Asset Health

## Context

2026-09 实测暴露了上传→asset→衍生链路的结构性脆弱（起因：外部拖入截图）：

- 上传返回 200 只代表“行写进去了”，不代表 asset 可用。checksum 去重曾长期返回一个原文件已删、衍生终态失败的死 asset，任何重传都 200 命中它，前端节点永久卡在 Preparing（已用窄修复止血：去重命中探活，缺失则退役死行后走新建）。
- 衍生是 push 式异步预计算（River + outbox + 轮询）：失败要靠覆盖式 error 字段取证，board 删除会留下孤儿 asset 行无限重试，前端对终态失败无错误输出。
- 职责重复：服务端预切 thumb/preview 的同时，前端 `useDisplayImagePreview` 也在客户端降采样。

根本问题：链路按“上游永远健康”假设写成，单点坏死会全链卡死且无法自愈。

## Decision

1. **Display Tier 采用 pull 按需渲染**：`thumb/preview/large-preview` 由 imgproxy 同步渲染，经 same-origin route 返回经过验证的图片 bytes。Workspace route 先验证归属和 Asset readiness，renderer 的签名 URL 仅供内部请求使用。Display Tier 不落数据库、不预生成，也不建立异步生成或修复队列。
2. **Processing Input 与 Display Tier 分离**：`processing-input` 表示 provider-neutral 的 `InputProfile`，不作为 Browser display endpoint，也不持久化 derivative rows。Tasking 通过 Media contract 解析输入引用；展示尺寸变化不改变处理输入的语义。
3. **Asset 健康显性化**：上传成功的 Asset 必须具有经过校验的原图事实。原图不可用、渲染失败和素材仍在准备中具有明确状态，调用方根据状态展示等待、错误或可重试操作，不把已失败的素材长期留在 Preparing。
4. **服务端 Tier 与客户端 preview 各有职责**：managed Asset 使用服务端 pull Tier；没有 Asset identity 的图片可以在 Browser 生成 preview。客户端 preview 的共享任务、使用期与 Object URL 回收由同一资源 owner 管理；decode gate 保留当前展示帧直到替换图片可用。
5. **Renderer source 策略显式且归 Media 所有**：Workspace Media 使用 `builtin-local`，通过 imgproxy sidecar 的只读 uploads volume 回源；Avatar 与 Branding Logo 使用 active Storage Profile 的 Local 或 S3-compatible source。HTTP adapter 保留授权、状态映射和 cache policy，Media renderer 拥有 source 选择、签名、bounded fetch 与图片响应校验。

## Consequences

- 正面：消灭整类“静默卡死”；去重、删除、衍生三处的隐式耦合收敛到一次同步检查；取证不再依赖被覆盖的 error 字段。
- 代价：首次 GET 需要承担同步渲染延迟与失败；API 承担 bounded image response 的内存和转发开销；S3 出站与 Browser preview 的算力开销需要监测。Board 删除与 Asset 生命周期的归属不由本 ADR 决定。
- 排除项：本 ADR 不改变 asset 跨 board 复用的用户级语义，不动计费/任务侧的媒体引用契约。

## Status

Accepted。Display Tier 通过 imgproxy pull rendering 与 same-origin bytes 交付；Media 持久化原图事实，Renderer 不写 derivative rows。

## Vendor Note (2026-09-14)

imgproxy vs Imagor 头对头结论：**维持 imgproxy，不换**。核对过双方一手文档：

- 缓存：调研曾称 imgproxy S3 结果缓存是 Pro 能力，经核官方文档有误——OSS 即支持 `IMGPROXY_CACHE_USE=s3` + 自定义 endpoint（点名 MinIO / R2 / DO Spaces），Imagor 在这点上没有实质优势。
- R2：双方都是 S3 兼容 endpoint 指过去；Imagor 只是文档里点名了 R2，imgproxy 需实测一次，无结构差异。
- 安全：双方都有 HMAC 签名 + 来源白名单 + image bomb 防护；Imagor 默认 SHA1，需显式切 sha256，imgproxy 默认即 SHA256。
- 形态：Imagor 可作 Go library 嵌入，但我们要的是 sidecar，这条反而是 imgproxy 的简单性占优。
- 社区与维护：imgproxy 11k star、双周发版、有商业公司背书；Imagor 4k、单维护者。libvips 同底，性能不会有量级差（Imagor 自跑 benchmark 也是同一量级，只能这么读）。
- 换 Imagor 的触发条件只剩：imgproxy 出现 license 变更、R2 实测不兼容，或需要 Result Storage 打标签做生命周期联动（Imagor 的 tagging + expiration 是它唯一独占的实用特性，但我们的生命周期由 DB 的 reclaim 拥有，价值有限）。
