# ADR-0007: Media Delivery Pull Model and Explicit Asset Health

## Context

Media Asset 的可用性取决于原图事实，Display Tier 是原图的展示投影。
把展示图生成作为 Asset 就绪的前置条件，会把渲染故障与原图生命周期耦合，并增加
队列、重试及派生状态的协调成本。展示尺寸与外部模型输入限制也必须各自独立。

## Decision

1. **Display Tier 采用 pull 按需渲染**：`thumb/preview/large-preview` 由 imgproxy 同步渲染，经 same-origin route 返回经过验证的图片 bytes。Workspace route 先验证归属和 Asset readiness，renderer 的签名 URL 仅供内部请求使用。Display Tier 不落数据库、不预生成，也不建立异步生成或修复队列。
2. **Processing Input 与 Display Tier 分离**：`processing-input` 表示 provider-neutral 的 `InputProfile`，不作为 Browser display endpoint，也不持久化 derivative rows。Tasking 通过 Media contract 解析输入引用；展示尺寸变化不改变处理输入的语义。
3. **Asset 健康显性化**：上传成功的 Asset 必须具有经过校验的原图事实。原图不可用、渲染失败和素材仍在准备中具有明确状态，调用方根据状态展示等待、错误或可重试操作，不把已失败的素材长期留在 Preparing。
4. **服务端 Tier 与客户端 preview 各有职责**：managed Asset 使用服务端 pull Tier；没有 Asset identity 的图片可以在 Browser 生成 preview。客户端 preview 的共享任务、使用期与 Object URL 回收由同一资源 owner 管理；decode gate 保留当前展示帧直到替换图片可用。
5. **Renderer source 策略显式且归 Media 所有**：Workspace Media 使用 `builtin-local`，通过 imgproxy sidecar 的只读 uploads volume 回源；Avatar 与 Branding Logo 使用 active Storage Profile 的 Local 或 S3-compatible source。HTTP adapter 保留授权、状态映射和 cache policy，Media renderer 拥有 source 选择、签名、bounded fetch 与图片响应校验。

## Consequences

- 正面：Asset 就绪不依赖异步展示图生成；原图生命周期与展示失败分开处理，调用方可以对实际读取结果给出明确反馈。
- 代价：首次 GET 需要承担同步渲染延迟与失败；API 承担 bounded image response 的内存和转发开销；S3 出站与 Browser preview 的算力开销需要监测。Board 删除与 Asset 生命周期的归属不由本 ADR 决定。
- 排除项：本 ADR 不改变 Media Asset 在同一 Workspace 内跨 Board 复用的语义，不改变 Task 的媒体引用或账务契约。

## Status

Accepted。Display Tier 通过 imgproxy pull rendering 与 same-origin bytes 交付；Media 持久化原图事实，Renderer 不写 derivative rows。

## Renderer 选型

采用 imgproxy sidecar，以内部 HMAC 签名请求、source allowlist 和图片大小限制约束
渲染访问。应用持有原图与回收事实，renderer 不拥有素材生命周期。Imagor 的嵌入式
library 或独立 result storage 不改善当前边界，因此不引入第二套 renderer。
