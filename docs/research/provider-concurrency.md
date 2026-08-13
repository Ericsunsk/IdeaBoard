# 图像生成 Provider 并发、速率限制与单请求输出研究

> 范围：Google Cloud Vertex AI Imagen 与 OpenAI Images API / Batch API。仅引用厂商官方文档；访问日期：2026-08-13。本文将**平台配额**、**单个 API 请求的多图输出**和**IdeaBoard 单个 Task 的内部并发**严格区分。

## 结论

1. `sampleCount`（Google）和 `n`（OpenAI）是**一次 API 请求要求返回多少张图**的参数，不是允许同时发出多少个请求的并发额度。
2. 平台限制必须以运行时凭据所在的 Google Cloud project / region 或 OpenAI organization / project 的实际 quota 为准；不能把文档示例的单请求上限换算成固定 worker 并发。
3. 两家公开文档均未为“一个业务 Task 内部应并行多少个图像请求”给出固定值。此值应是应用层调度策略：受 provider 实际余量、单次请求的输出数、延迟、`429`/资源耗尽错误和账户隔离共同约束。

## 概念边界

| 层级 | Google Vertex AI Imagen | OpenAI | 对 Task 调度的含义 |
| --- | --- | --- | --- |
| 单 API 请求的输出数 | `parameters.sampleCount`；图像数量接受 `1` 到 `4`。 | Image API 的 `n` 可在一个 request 中生成多张图；默认返回一张。 | 可优先用一个 request 的多图输出满足一个 Task 的 `outputCount`，但它仍只是一条 in-flight request。 |
| 平台速率 / 容量 | 配额按 project、region、model（或相应 quota metric）定义；在 Cloud Console 查看 / 调整。 | 使用 RPM、TPM、IPM 等；限制按 organization 与 project、按 model 定义，可在 Limits 页面查看。 | 全局调度器必须按实际 credential + model（Google 还需按 region）共享容量，不能只按用户或 Task 各自计数。 |
| 异步批处理 | 本次 Imagen 单请求文档未把 `sampleCount` 描述为 Batch job。 | Batch API 支持 `/v1/images/generations` 和 `/v1/images/edits`，有独立速率池与 24 小时 completion window。 | 交互式 Canvas Task 走同步 API；可容忍延迟的离线批量任务才考虑 Batch API。 |
| 单 Task 内部并发 | 无厂商固定建议。 | 无厂商固定建议。 | 这是本系统的可配置工作策略，不应伪装为 provider quota。 |

## Google Cloud Vertex AI Imagen

### 官方事实

- Imagen 文本生成请求以 `parameters.sampleCount` 指定 `IMAGE_COUNT`；文档明确说明可接受范围为 `1` 到 `4`。`sampleCount: 2` 的示例响应会返回两个 prediction 对象。因此它表达的是**一条 `predict` 请求内的样本数量**，不是 four-way request concurrency。
  来源：[Generate images using text prompts with Imagen on Vertex AI](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/image/generate-images)
- Google 的 Generative AI quota 文档说明页面按 region 和 model 列出 quota，并说明可在 Google Cloud Console 查看和编辑配额。对于所列的 `generateContent` 多模态输入 quota，范围是“给定 project 的每个 region”，并按 base model / resolution 实施；这说明配置与观测必须保留 project、region、model 维度，不能使用通用固定并发常数。
  来源：[Generative AI quotas and system limits](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/quotas)
- 该配额页的 Gemini Batch inference 段落还明确：没有预定义的 concurrent batch inference job quota，而是共享资源池；容量饱和时 Batch request 可能排队。该规则是 Gemini Batch 的说明，**不能外推成 Imagen 同步 `predict` 的并发承诺**。
  来源：[Generative AI quotas and system limits](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/quotas#batch-inference)

### 调度含义

- 对一个需要 1--4 张同规格结果的 Task，优先一次 Imagen request 设置对应的 `sampleCount`；不要为每张图无条件创建独立并行 request。
- `sampleCount=4` 会使该 Task 有一条 in-flight API request、至多四张输出；应用调度计数应同时记录 `in_flight_requests` 和 `requested_images`，不能只计其中一个。
- 具体并发槽位必须从目标 project/region/model 的 Cloud Console quota、运行时成功率与延迟中计算；官方页面未给出适用于所有 Imagen 项目的固定槽位数。

## OpenAI Images API

### 官方事实

- OpenAI 图像生成指南说明 Image API 的 Generations endpoint 可从 prompt 生成图片，且可以通过 `n` 参数在单个 request 中生成多张；未设置时 API 返回一张。由此，`n` 是输出样本数，不是 API client 的并发控制参数。
  来源：[Image generation](https://developers.openai.com/api/docs/guides/image-generation)
- OpenAI rate limits 可使用 RPM、RPD、TPM、TPD 和 **IPM（images per minute）** 等指标，先触及任一指标即受限。限制在 organization 和 project 层定义、按 model 变化，部分 model family 还共享限制；实际值在 Limits 页面查看。
  来源：[Rate limits](https://developers.openai.com/api/docs/guides/rate-limits)
- HTTP response 可能含 `x-ratelimit-limit-*`、`x-ratelimit-remaining-*`、`x-ratelimit-reset-*`；临时 rate-limit `429` 可含 `Retry-After`。官方建议至少等待该值并加随机延迟；官方 SDK 会为符合条件的 rate-limit error 自动重试并遵守 `Retry-After`。
  来源：[Rate limits](https://developers.openai.com/api/docs/guides/rate-limits#rate-limits-in-headers)
- Batch API 支持 `/v1/images/generations` 与 `/v1/images/edits`。Batch 使用与同步 per-model limits 分离的 rate-limit pool，完成窗口为 24 小时；每个 Batch 最多 50,000 requests、输入文件最多 200 MB、创建上限为每小时 2,000 个 Batch，排队 prompt token 上限仍以 model 实际 Limits 页面为准。
  来源：[Batch API](https://developers.openai.com/api/docs/guides/batch)

### 调度含义

- 同步 Task 的容量同时受 request 速率和 **IPM** 约束。仅依据 RPM 配置 request worker concurrency，可能仍会因较大的 `n` 触及 IPM。
- 遇到 `429` 时，按 `Retry-After`（若存在）做带 jitter 的退避；不要用紧密重试来“提高吞吐”，因为失败请求也会计入 per-minute limit。
- Batch 的“独立、更高限额”不等同于低延迟交互服务，且其 24 小时窗口不符合用户等待 Canvas 结果的体验。只有明确为离线处理且可接受异步结果时使用。

## 单个 Task 内部并发建议（工程推论，非官方配额）

以下为保守、可逆的应用策略；它不是 Google 或 OpenAI 发布的数值承诺：

1. 把 provider 调用并发与结果数量解耦：一个 Task 内优先通过一次 request 的 `sampleCount` / `n` 取得多图，而不是按 `outputCount` 直接并发多条 request。
2. 初始设为同一 `account + model` 的单 Task 内最多 **1 条 in-flight provider request**；需要多阶段或 provider 不支持所需 output count 时，再经共享的 account/model semaphore 排队。这是为避免一个 Task 绕过全局账户配额的保守起点，不是平台要求。
3. 只在可观测数据证明有余量后提高该值：观测 actual quota/header、Google quota metric、p50/p95 latency、`429` / resource-exhausted 比率及单位时间生成图片数。提高时必须让多个 Task 竞争同一 credential 的共享预算。
4. 为瞬态 `429`、`5xx` 和资源不足设置有上限的 exponential backoff + jitter；可修正的 prompt / safety / 输入错误不自动重试。对重试，计入相同 Task 的总 deadline 与尝试上限。

## 落地检查清单

- OpenAI：部署前记录组织 / project / model 的 RPM 与 IPM，并在 response headers 中记录 remaining 与 reset；为共享限额的 model family 使用同一个 limiter。
- Google：部署前记录 project、region、Imagen model 的实际 quota / usage；若 provider 用 global endpoint，也应按官方 metric 的 scope 建立独立 bucket。
- 将 `requested_images`（`sampleCount` / `n`）和 `provider_requests` 分开计量；前者用于 IPM / 成本 / 产出，后者用于 in-flight slot 和 RPM。
- 将上述并发初始值做成 provider account/model 配置而非编译期常数，并保留将值降回 `1` 的运行时 fallback。

## 未从官方文档获得的内容

- 未找到适用于所有 Google Vertex AI Imagen account 的固定 requests-per-minute 或 simultaneous-request 数字；Google 指向按 project、region、model 的 Console quota，故本文不杜撰通用值。
- 未找到 OpenAI 为单一业务 Task 规定的固定 Images API 并发数；OpenAI 提供 IPM/RPM 等配额和 rate-limit response headers，具体值取决于 organization/project、usage tier 与 model。
- `sampleCount`/`n` 的单请求输出上限不应被当作账户的平台配额，也不应据此推导 worker 数。
