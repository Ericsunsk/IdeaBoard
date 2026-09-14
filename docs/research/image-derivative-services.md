# Image Derivative Services 调研：imgproxy 之外的 pull 按需渲染方案

> 范围：为 [ADR-0007](../adr/0007-media-delivery-pull-model.md) 的 pull 按需渲染方向，评估 imgproxy 之外的成熟替代方案。
> 方法：只认一手来源（官方文档、GitHub 仓库、官方 benchmark、源码）；每个事实性 claim 均带行内链接；凡引用 benchmark 均说明是谁跑的、怎么跑的。
> 访问日期：2026-09-14。Star / issue / release 状态均为该日期抓取到的页面值，随时间漂移。
> 项目约束（评估标尺）：Go + Gin 自托管、本地盘（builtin-local）现在可用、未来迁 Cloudflare R2（S3 协议）；按需缩放/格式转换（thumb / preview / processing 档）；签名 URL 防盗刷；webp / avif 输出；最好有 Prometheus 指标；偏好自托管、开源、Go 生态亲和。托管方案重点看 R2 协同、成本、数据出境、与 storage profile 体系的冲突。Media / Storage 的归属契约见 [DESIGN.md](../../DESIGN.md)（第 7 条：Media 拥有 provider / profile / upload serving / 引用边界）。

## 结论（先行）

1. **没有方案显著优于 imgproxy，应继续以 imgproxy（基准 [v4.0.14](https://github.com/imgproxy/imgproxy/releases)，releases 页标为 Latest）为 pull 渲染默认选型**：它是本轮唯一同时满足“本地盘源站现在可用、S3 源站未来可用、签名 URL、webp + avif 输出、Prometheus 指标、Go 自托管、release 高频（v4.0.14 页面显示 24 Aug 发布）”的候选，且项目 spike 已验证可用（项目内部前提）。
2. **唯一值得立项时再做一次小规模对照的候补是 Imagor**：同样 Go + libvips、自托管开源，在 [imagor 官方 benchmark 页](https://docs.imagor.net/benchmarks) 自跑的对照里与 imgproxy 同一量级（部分格式互有领先，见正文），且 [S3 文档明确点名兼容 Cloudflare R2](https://docs.imagor.net/storage-s3)。但它不是“显著更优”，只是“R2 迁移时值得复测一次”的候补。
3. 其余候选均有至少一条硬伤与项目约束直接冲突：Thumbor（Python 栈、Pillow 管线在本轮唯一的量化对照里全面落后）、Picfit（官方 README 的格式表无 AVIF）、willnorris/imageproxy（WebP 仅 decode、纯 Go 无 libvips，加 AVIF 等于自研）、Cloudflare Images / Bunny Optimizer（把变换所有权搬出自托管体系，与 storage profile 契约冲突，且计费与数据主权另起一摊）。

## 对比表

说明：✅ = 一手来源明确支持；⚠️ = 需读源码/另行核实；❌ = 一手来源显示不支持或缺失。“Go 亲和”为工程推论，非厂商声明。

| 维度 | [imgproxy](https://github.com/imgproxy/imgproxy)（基准） | [Imagor](https://github.com/cshum/imagor) | [Thumbor](https://github.com/thumbor/thumbor) | [Picfit](https://github.com/thoas/picfit) | [willnorris/imageproxy](https://github.com/willnorris/imageproxy) | [Cloudflare Images](https://developers.cloudflare.com/images/) | [Bunny Optimizer](https://bunny.net/docs/optimizer/dynamic-images/overview) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 语言 / 引擎 | Go + [libvips](https://github.com/imgproxy/imgproxy)（README 自述） | Go + [libvips 经 vipsgen 绑定](https://github.com/cshum/imagor)（README 自述） | Python（仓库语言分布 + 部署形态，见 [README](https://github.com/thumbor/thumbor) pip 安装节） | Go（[README](https://github.com/thoas/picfit) 自述 reusable Go server） | 纯 Go（[README 自述 easy deployment, since it's pure go](https://github.com/willnorris/imageproxy)） | 托管（Cloudflare edge） | 托管（Bunny CDN edge，见 [Dynamic Images](https://bunny.net/docs/optimizer/dynamic-images/overview) 首段缓存描述） |
| License | [Apache-2.0](https://github.com/imgproxy/imgproxy)（仓库 license 栏） | [Apache-2.0](https://github.com/cshum/imagor)（仓库 license 栏） | [MIT](https://github.com/thumbor/thumbor)（仓库 license 栏） | [MIT](https://github.com/thoas/picfit)（仓库 license 栏） | [Apache 2.0](https://github.com/willnorris/imageproxy)（README License 节） | 商业闭源托管 | 商业闭源托管 |
| Star / Fork（抓取日） | [11.1k / 775](https://github.com/imgproxy/imgproxy) | [4.0k / 175](https://github.com/cshum/imagor) | [10.5k / 862](https://github.com/thumbor/thumbor) | [2.3k / 188](https://github.com/thoas/picfit) | [4.0k / 513](https://github.com/willnorris/imageproxy) | 不适用 | 不适用 |
| Open issues / PRs（抓取日） | [61 / 13](https://github.com/imgproxy/imgproxy) | [2 / 0](https://github.com/cshum/imagor) | [14 / 22](https://github.com/thumbor/thumbor) | [10 / 2](https://github.com/thoas/picfit) | [53 / 10](https://github.com/willnorris/imageproxy) | 不适用 | 不适用 |
| 最新 release（抓取日页面值） | [v4.0.14，Latest，页面显示 24 Aug](https://github.com/imgproxy/imgproxy/releases) | [v1.9.6，Latest，页面显示 25 Aug](https://github.com/cshum/imagor/releases) | [7.8.0，Latest，页面显示 30 May（含安全修复）](https://github.com/thumbor/thumbor/releases) | release 列表顶部为 [0.16.1](https://github.com/thoas/picfit/releases) | [v0.13.0，Latest，页面显示 29 Jun](https://github.com/willnorris/imageproxy/releases)；注意 [v0.12.0 说明写了时隔三年多才发版](https://github.com/willnorris/imageproxy/releases) | 不适用 | 不适用 |
| 缩放 / 档位映射 | ✅ processing options + [presets](https://docs.imgproxy.net/usage/presets)（thumb / preview / processing 可收敛为 preset 名，推论） | ✅ endpoint + [filters 管线](https://docs.imagor.net/filters)（档位可收敛为签名路径模板，推论） | ✅ 路径参数 + filters（[README 示例](https://github.com/thumbor/thumbor)） | ✅ resize / thumbnail / crop 等 [Operations](https://github.com/thoas/picfit) | ✅ options 列表（[README URL Structure](https://github.com/willnorris/imageproxy)） | ✅ width / height / fit / gravity 等 [Features 参数表](https://developers.cloudflare.com/images/optimization/features/) | ✅ width / height / crop / face_crop 等 [Dynamic Images](https://bunny.net/docs/optimizer/dynamic-images/overview) |
| webp 输出 | ✅ [格式表 source + result 均为 ✔](https://docs.imgproxy.net/image_formats_support) | ✅ [format filter 接受 webp](https://docs.imagor.net/filters) | 一手页面未逐项核实（Pillow 管线，见 benchmark 节） | ✅ [Formats 节含 webp](https://github.com/thoas/picfit) | ⚠️ README 写的是 [webp（decode only）](https://github.com/willnorris/imageproxy)，输出侧未声明 webp encode | ✅ [format 接受 webp](https://developers.cloudflare.com/images/optimization/features/) | ✅ 变换卡片列出 [JPEG / PNG / WebP / GIF 互转](https://bunny.net/docs/optimizer/dynamic-images/overview)；另有 [AVIF 输出面积上限 4MP](https://docs.bunny.net/optimizer/limits) |
| avif 输出 | ✅ [格式表 source + result 均为 ✔](https://docs.imgproxy.net/image_formats_support) | ✅ [format filter 接受 avif](https://docs.imagor.net/filters) | ⚠️ [7.7.2 起有 auto heif / avif 的 Vary 处理](https://github.com/thumbor/thumbor/releases)，[7.8.0 依赖含 pillow-heif](https://github.com/thumbor/thumbor/releases)，输出矩阵未逐项核实 | ❌ [Formats 节仅 jpeg / png / gif / bmp / webp](https://github.com/thoas/picfit)，无 AVIF | ❌ README 格式行仅 [jpeg / png / webp（decode only）/ tiff / gif](https://github.com/willnorris/imageproxy)，无 AVIF | ✅ [format 接受 avif](https://developers.cloudflare.com/images/optimization/features/)（并注明 AVIF 编码慢、大图回退 WebP/JPEG） | ⚠️ [AVIF 输出面积上限 4MP](https://docs.bunny.net/optimizer/limits)，超限行为未核实 |
| 签名 URL 防盗刷 | ✅ [key + salt 的 URL 签名，HMAC-SHA256 + URL-safe Base64](https://docs.imgproxy.net/usage/signing_url)，另有 [allowed sources 白名单](https://docs.imgproxy.net/configuration/options) | ✅ [IMAGOR_SECRET 签名，默认 SHA1 并建议 sha256/sha512](https://docs.imagor.net/security)，另有 [expire 过期 filter](https://docs.imagor.net/security) 与 [allowed sources](https://docs.imagor.net/security) | ✅ [SECURITY_KEY 的 HMAC-SHA1 + urlsafe_b64encode 签名](https://thumbor.readthedocs.io/en/latest/security.html) | ✅ [secret_key 的 HMAC-SHA1 + sig 参数](https://github.com/thoas/picfit) | ✅ [HMAC-SHA256 签名 + signatureKey，支持 vu 过期时间戳](https://github.com/willnorris/imageproxy) | ⚠️ 本轮抓取 [optimization/security 路径返回 404](https://developers.cloudflare.com/images/optimization/security/)，未确认与需求同义的签名机制，需另行核实 | ⚠️ 未核实（Dynamic Images 为公开 query 参数形态，见 [overview](https://bunny.net/docs/optimizer/dynamic-images/overview)），需另行核实 token / 签名能力 |
| 本地盘源站现在可用 | ✅ [IMGPROXY_LOCAL_FILESYSTEM_ROOT + local://](https://docs.imgproxy.net/image_sources/local_files) | ✅ [FILE_LOADER_BASE_DIR / FILE_STORAGE_BASE_DIR](https://docs.imagor.net/configuration) | ✅ file loader（[7.8.0 修复 file_loader 路径逃逸](https://github.com/thumbor/thumbor/releases) 反证该能力存在） | ✅ [src type fs / http+fs](https://github.com/thoas/picfit) | ✅ 磁盘目录 cache（[README Cache 节](https://github.com/willnorris/imageproxy)；注意这是结果缓存而非源站挂载语义） | ❌ 需经远程 origin 拉取（[Transformations 结构为 ZONE + SOURCE-IMAGE](https://developers.cloudflare.com/images/optimization/features/)），本地盘直挂不在模型内 | ❌ 经 Pull Zone 回源（[Dynamic Images 需 Pull Zone + Optimizer](https://bunny.net/docs/optimizer/dynamic-images/overview)），本地盘直挂不在模型内 |
| S3 / R2 源站未来可用 | ✅ [IMGPROXY_USE_S3 + s3:// + 自定义 endpoint（含 MinIO 示例）](https://docs.imgproxy.net/image_sources/amazon_s3)；R2 未在该页点名，S3 兼容层面推论可用 | ✅ [S3 Loader / Storage / Result Storage + 自定义 endpoint，文档明确点名 Cloudflare R2 / MinIO / DO Spaces](https://docs.imagor.net/storage-s3) | ✅ README 自述 [支持 S3 等多种 storage](https://github.com/thumbor/thumbor)（R2 兼容性未逐项核实） | ✅ [src/dst type s3（含 endpoint 字段）](https://github.com/thoas/picfit)（R2 兼容性未逐项核实） | ✅ [s3:// cache + endpoint / path-style 参数（文档以 MinIO / DO Spaces 为例）](https://github.com/willnorris/imageproxy)（R2 未点名，S3 兼容层面推论） | ✅ [R2 即官方示例中的外部存储](https://developers.cloudflare.com/images/pricing/)（定价页示例直接按 Images + R2 加总） | ⚠️ 回源可指 S3 类源（未在本轮一手抓取中核实 R2 细节，需另行核实） |
| Prometheus 指标 | ✅ [IMGPROXY_PROMETHEUS_BIND + namespace，指标含 requests_total / errors_total / latency histogram / vips 内存等](https://docs.imgproxy.net/monitoring/prometheus) | ✅ [PROMETHEUS_BIND / PROMETHEUS_PATH / PROMETHEUS_NAMESPACE](https://docs.imagor.net/configuration) | ⚠️ [7.7.5 修复 prometheus registry 重名异常](https://github.com/thumbor/thumbor/releases) 反证有 Prometheus 集成，指标清单未核实 | ⚠️ 仓库根目录存在 [metrics.go](https://github.com/thoas/picfit)（文件列表），是否为 Prometheus exposition 需读源码确认；另有 [stats / health / pprof 开关](https://github.com/thoas/picfit) | ⚠️ 仓库根目录存在 [metrics.go](https://github.com/willnorris/imageproxy)（文件列表），是否为 Prometheus exposition 需读源码确认 | 托管侧自带分析（本项目 Prometheus 需求不适用，推论） | 托管侧自带监控（本项目 Prometheus 需求不适用，推论） |
| Go 生态亲和（推论） | 高：Go 服务，spike 已验证 | 高：Go 服务 + 可作 Go library 嵌入（README 自述 server and Go library） | 低：Python 服务，团队无 Python 运行时资产 | 中：Go 服务，但 AVIF 缺口致命 | 中高：纯 Go 无 cgo，部署轻，但图像能力弱 | 低：逻辑外包，需 Workers / API 胶水 | 低：逻辑外包，需 Pull Zone / CDN 胶水 |

## imgproxy（基准，v4.0.14）

- 现状：[releases 页 v4.0.14 为 Latest](https://github.com/imgproxy/imgproxy/releases)；仓库 [11.1k star / 775 fork，open issues 61 / PRs 13](https://github.com/imgproxy/imgproxy)，license 为 [Apache-2.0](https://github.com/imgproxy/imgproxy)；README 自述 [Go + libvips，URL 签名、安全（image bomb 检查、来源限制）为设计原则](https://github.com/imgproxy/imgproxy)。
- 签名：[key + salt，HMAC-SHA256，URL-safe Base64，文档给出完整验算示例](https://docs.imgproxy.net/usage/signing_url)；另有 [ALLOWED_SOURCES 前缀白名单、SSRF 方向的 loopback / link-local / private 地址开关](https://docs.imgproxy.net/configuration/options)。
- 源站：本地盘走 [IMGPROXY_LOCAL_FILESYSTEM_ROOT + local://](https://docs.imgproxy.net/image_sources/local_files)；S3 走 [IMGPROXY_USE_S3 + s3:// + 自定义 endpoint](https://docs.imgproxy.net/image_sources/amazon_s3)。R2 迁移时对应把 endpoint 指向 R2 S3 兼容 endpoint 做验证即可（兼容性推论，需实测）。
- 格式：[PNG / JPEG / JXL / WebP / AVIF / GIF 等 source + result 均为 ✔](https://docs.imgproxy.net/image_formats_support)；另有 [AUTO_WEBP / AUTO_AVIF / AUTO_JXL 的 Accept 协商开关](https://docs.imgproxy.net/configuration/options)。thumb / preview / processing 三档可收敛为 [presets](https://docs.imgproxy.net/usage/presets)（映射方式为工程推论）。
- 指标：[Prometheus（bind + namespace），指标含 requests_total / status_codes_total / errors_total / request_duration_seconds 等及 vips 内存指标](https://docs.imgproxy.net/monitoring/prometheus)。
- 注意（限制，非缺点）：[内部结果缓存（IMGPROXY_CACHE_USE 等）标记为 Pro 能力](https://docs.imgproxy.net/configuration/options)，OSS 侧不要把“结果对象缓存”规划进 imgproxy 本体，应由 CDN / 反代缓存层承担（架构推论）。这恰好与 ADR-0007 的 pull + 缓存方向一致：缓存键与 TTL 放在自有层，imgproxy 只做无状态渲染。

## Imagor（唯一值得复测的候补）

- 现状：[4.0k star / 175 fork，open issues 2 / PRs 0](https://github.com/cshum/imagor)，license [Apache-2.0](https://github.com/cshum/imagor)；[v1.9.6 为 Latest（页面显示 25 Aug）](https://github.com/cshum/imagor/releases)，release 节奏密（5 月之后 v1.9.1 → v1.9.6）；README 自述 [Go + libvips（vipsgen 绑定）+ streaming，兼容 thumbor URL 语法](https://github.com/cshum/imagor)，且 [可作 Go library 嵌入](https://github.com/cshum/imagor)——这是本轮唯一“可嵌进 Gin 进程”的选项（能力引述自 README，是否采用是架构决策）。
- 签名：[IMAGOR_SECRET 全量签名，默认 SHA1（与 thumbor 同算法），官方明确 SHA1 已不算安全并给出 sha256 / sha512 + 截断选项](https://docs.imagor.net/security)；[expire(timestamp) 过期 filter](https://docs.imagor.net/security) 对应防盗刷的时效需求；[allowed sources（glob / regexp / base URL / HTTPS-only）](https://docs.imagor.net/security)；[VIPS 分辨率上限、request/load/process 超时与并发队列（满时 429）](https://docs.imagor.net/security)。
- 源站：[File Loader / Storage / Result Storage 三件套](https://docs.imagor.net/configuration)；[S3 章节明确点名兼容 Cloudflare R2 / MinIO / DO Spaces，支持 S3_ENDPOINT + S3_FORCE_PATH_STYLE](https://docs.imagor.net/storage-s3)——R2 亲和是本轮自托管候选中文档最直白的一家。
- 格式：[format filter 接受 jpeg / png / gif / webp / avif / jxl / tiff / jp2](https://docs.imagor.net/filters)；[IMAGOR_AUTO_WEBP / IMAGOR_AUTO_AVIF 自动协商](https://docs.imagor.net/configuration)。
- 指标：[PROMETHEUS_BIND / PROMETHEUS_PATH / PROMETHEUS_NAMESPACE](https://docs.imagor.net/configuration)。
- Benchmark（是谁跑的、怎么跑的）：**跑方是 imagor 作者方**，数据出自 [imagor 官方 benchmark 页](https://docs.imagor.net/benchmarks)，源头是公开的 [image-servers-benchmark](https://github.com/cshum/image-servers-benchmark) 项目及其 committed results；最后更新标注 May 2026；环境 [AWS EC2 c7i.large，2 VUs，5m，10s warmup，resize 到 512x512](https://docs.imagor.net/benchmarks)；对照版本为 [imagor 1.9.1（ghcr.io/cshum/imagor:1.9.1）vs imgproxy v4.0.3（darthsim/imgproxy:latest）vs thumbor 7.7.7](https://docs.imagor.net/benchmarks)。代表性吞吐（req/s）：JPEG [74.35 / 82.01 / 59.45](https://docs.imagor.net/benchmarks)；PNG [18.56 / 17.35 / 10.23](https://docs.imagor.net/benchmarks)；WebP [21.23 / 24.27 / 15.80](https://docs.imagor.net/benchmarks)；AVIF [17.77 / 17.15 / 14.85](https://docs.imagor.net/benchmarks)。官方自己的 FAQ 结论是：[该集合上 imagor 全面快于 thumbor，但与 imgproxy 互有胜负（imagor 领先 PNG / AVIF，imgproxy 领先 JPEG / WebP），且 AVIF 结论强依赖 encoder 设置](https://docs.imagor.net/benchmarks)。因此**不得引申为“Imagor 更快”**；只能说同一量级，R2 迁移时用自己的图集复测。
- 与 imgproxy 的实质差异（推论，需实测）：Imagor 的 result storage 语义更贴“渲染后落盘”；若未来希望衍生对象进 R2 并被 storage profile 体系追踪，Imagor 的 [S3 Result Storage](https://docs.imagor.net/storage-s3) 比 imgproxy OSS（结果缓存为 Pro）更顺。但代价是多一套签名/路径体系与运维面，收益不足以现在切换。

## Thumbor（不推荐：栈与性能双冲突）

- 现状：[10.5k star / 862 fork，open issues 14 / PRs 22](https://github.com/thumbor/thumbor)，license [MIT](https://github.com/thumbor/thumbor)；[7.8.0 为 Latest（页面显示 30 May），含多项安全修复（file_loader 路径逃逸、签名绕过、ALLOWED_SOURCES 加固、filter DoS），官方强烈建议生产升级](https://github.com/thumbor/thumbor/releases)。
- 签名：[SECURITY_KEY 的 HMAC-SHA1 + urlsafe_b64encode](https://thumbor.readthedocs.io/en/latest/security.html)。注意算法为 SHA1（文档原文），低于 imgproxy（SHA256）与 Imagor 可选的 sha256/512（对比推论）。
- 性能：在本轮唯一可引用的量化对照（即上节 imagor 官方 benchmark，May 2026，c7i.large，512x512 resize）中，[thumbor 7.7.7 四项格式全面落后于 imagor 与 imgproxy](https://docs.imagor.net/benchmarks)；官方页将原因归于 [Pillow 管线 vs libvips 管线](https://docs.imagor.net/benchmarks)。引用时必须带该限定语，不作泛化断言。
- 否决点：Python 运行时与团队 Go 资产不亲和（推论）；[7.8.0 刚修过签名绕过与路径逃逸](https://github.com/thumbor/thumbor/releases)，引入即背安全审计负担；AVIF/HEIF 侧只有 [Vary 处理](https://github.com/thumbor/thumbor/releases) 与 [pillow-heif 依赖](https://github.com/thumbor/thumbor/releases) 的间接证据，未见逐项输出矩阵。

## Picfit（不推荐：AVIF 缺口一票否决）

- 现状：仓库为 [thoas/picfit](https://github.com/thoas/picfit)（注意不是已 404 的 pilu/picfit 猜测地址）；[2.3k star / 188 fork，open issues 10 / PRs 2](https://github.com/thoas/picfit)，license [MIT](https://github.com/thoas/picfit)；[release 列表顶部为 0.16.1](https://github.com/thoas/picfit/releases)。
- 能力面：[src 支持 fs / http+fs / s3（含 endpoint）/ dos3 / gcs](https://github.com/thoas/picfit)；[kvstore 支持 redis / cache / redis-cluster / redis-roundrobin](https://github.com/thoas/picfit)；签名为 [secret_key 的 HMAC-SHA1 + sig 参数](https://github.com/thoas/picfit)；另有 [allowed_sizes 与 max_image_dimensions、并发 semaphore、stats / health / pprof 开关](https://github.com/thoas/picfit)。
- 否决点：[Formats 节仅列 jpeg / png / gif / bmp / webp](https://github.com/thoas/picfit)，无 AVIF——与“webp/avif 输出”硬需求直接冲突。Prometheus 方面仅见仓库根目录 [metrics.go](https://github.com/thoas/picfit) 文件存在，无一手文档说明其 exposition 格式，需读源码（未断言支持）。

## willnorris/imageproxy（不推荐：能力天花板太低）

- 现状：[4.0k star / 513 fork，open issues 53 / PRs 10](https://github.com/willnorris/imageproxy)，个人项目属性强（README 以第一人称自述用途）；[v0.13.0 为 Latest（页面显示 29 Jun）](https://github.com/willnorris/imageproxy/releases)，但 [v0.12.0 自述是三年多来的首个 release](https://github.com/willnorris/imageproxy/releases)，维护节律弱于 imgproxy / Imagor（节律对比为推论）。
- 能力面：[resize / crop / rotation 基础变换](https://github.com/willnorris/imageproxy)；签名为 [HMAC-SHA256 + signatureKey，多 key 轮换，vu 过期时间戳](https://github.com/willnorris/imageproxy)，签名设计本身不差；缓存支持 [memory / 磁盘目录 / S3（含 endpoint / path-style 参数，示例为 MinIO / DO Spaces）/ GCS / Azure / Redis](https://github.com/willnorris/imageproxy)。
- 否决点：[格式行仅 jpeg / png / webp（decode only）/ tiff / gif（含 animated gif）](https://github.com/willnorris/imageproxy)——WebP 只能解不能编、无 AVIF；且纯 Go 无 libvips（[README 自述 pure go](https://github.com/willnorris/imageproxy)），补 AVIF/高性能管线等于自研。Prometheus 方面仅见仓库根目录 [metrics.go](https://github.com/willnorris/imageproxy) 文件存在，无一手文档说明，需读源码（未断言支持）。

## Cloudflare Images（不推荐为默认路径；仅当“不想运维”时备选）

- 模型：[两种接入：自带存储（含 R2 等 S3 兼容）做变换，或上传到 Images 全托管](https://developers.cloudflare.com/images/)；URL 结构为 [/cdn-cgi/image/OPTIONS/SOURCE-IMAGE（远程）或 imagedelivery.net 交付 URL（托管）](https://developers.cloudflare.com/images/optimization/features/)。
- 能力面：[width / height / fit（含 scale-down / contain / cover / crop / pad 等）/ gravity（含 auto / face）/ rotate / blur / sharpen / quality / format（auto / avif / webp / jpeg…）](https://developers.cloudflare.com/images/optimization/features/)；[format=auto 按一次计费，多份投递不重复计](https://developers.cloudflare.com/images/pricing/)。
- 成本（一手）：[Free 计划每月 5,000 unique transformations，超限新变换返回 9422 错误且不收费](https://developers.cloudflare.com/images/pricing/)；[Paid 为首 5,000 含 + $0.50 / 1,000 unique transformations，存储 $5 / 100,000 张 / 月，交付 $1 / 100,000 次 / 月](https://developers.cloudflare.com/images/pricing/)；[官方示例：R2 存 5,000 张×5MB + 2,000 张×5 档变换，月估 $2.72（Images $2.50 + R2 $0.22）](https://developers.cloudflare.com/images/pricing/)。注意这是官方示例口径，实际以用量为准；R2 部分按 [R2 定价](https://developers.cloudflare.com/r2/pricing/) 另算（该页数值本轮未抓取，不转述）。
- 否决/风险点：
  1. **防盗刷未核实**：本轮抓取 [optimization/security 路径返回 404](https://developers.cloudflare.com/images/optimization/security/)，未确认与需求同义的 URL 签名机制；且 [blur 文档明确警告 URL 参数可被改掉、不宜作内容遮蔽依赖](https://developers.cloudflare.com/images/optimization/features/)——URL 可变性是该模型固有特征，签名方案必须另行核实后才能对标 imgproxy。
  2. **与 storage profile 体系冲突**（架构推论）：变换所有权搬到 Cloudflare 侧后，provider / profile / 引用边界（[DESIGN.md](../../DESIGN.md) 第 7 条）将被掏空为“透传 + 缓存键管理”，且本地盘源站不在模型内（需先解决 origin 可达）。
  3. **数据出境/主权未评估**：本轮未抓取 Cloudflare 区域 / R2 jurisdiction 一手页，不代结论；若用户素材涉敏感行业，这是立项前置核查项（风险提示，非事实断言）。

## Bunny Optimizer（不推荐；比 Cloudflare 更不贴）

- 模型：[Pull Zone 上启用 Optimizer (+ Dynamic Image API)，query 参数实时变换，边缘缓存，purge 原图连带清掉各变换](https://bunny.net/docs/optimizer/dynamic-images/overview)。
- 能力面：[width / height / crop / aspect_ratio / face  crop / brightness / gamma / sharpen / quality / format 等](https://bunny.net/docs/optimizer/dynamic-images/overview)，变换顺序为 [crop → resize → 翻转旋转 → 滤镜 → 颜色 → 格式质量](https://bunny.net/docs/optimizer/dynamic-images/overview)。
- 成本（一手）：[每 Pull Zone $9.50 / 月，含 unlimited optimisation / requests / transformations，CDN 带宽另计](https://docs.bunny.net/optimizer/pricing)。
- 否决/风险点：
  1. **格式天花板存疑**：变换卡片只列 [JPEG / PNG / WebP / GIF 互转](https://bunny.net/docs/optimizer/dynamic-images/overview)；AVIF 仅在 [limits 页出现“AVIF 输出面积上限 4MP”](https://docs.bunny.net/optimizer/limits)，超限行为与编码质量档未核实——processing 档若有大图 AVIF 需求，这是硬风险。
  2. **签名未核实**：Dynamic Images 为公开 query 参数形态（见 [overview 示例](https://bunny.net/docs/optimizer/dynamic-images/overview)），token / 签名能力需另行核实。
  3. **生态错位**（推论）：团队已在 Cloudflare Pages + 未来 R2 上下注，再引入 Bunny 等于第二家 CDN / 计费 / 主权面；除非同时把 CDN 从 Cloudflare 搬走，否则不划算。

## 最终推荐

- **默认选型：imgproxy（维持基准）**。它是在本轮一手证据下唯一“六边形”满足约束的选项，且 spike 已验证可用；Imagor 的 benchmark 也只证明“同一量级”，不构成切换理由。
- **Imagor 为候补，触发复测而非现在切换**：R2 迁移立项时，用本项目真实图集（小图 thumb、大图 preview、AI 输入 processing-input 三档 + AVIF 输出）对 imgproxy vs Imagor 做一次同机复测，以 p50/p95 首渲延迟与失败率为准。
- **imgproxy 何时该被换掉（触发条件）**：
  1. 需要 OSS 本体内的“渲染后落 R2 并纳入对象生命周期/追踪”，且不愿为此买 imgproxy Pro（[结果缓存 S3 类后端为 Pro 能力](https://docs.imgproxy.net/configuration/options)）→ 换 Imagor（[S3 Result Storage OSS 即有](https://docs.imagor.net/storage-s3)）。
  2. 需要把渲染服务以 Go library 嵌进现有 Gin 进程（减少一个 sidecar）→ 换 Imagor（[README 自述 server and Go library](https://github.com/cshum/imagor)）。
  3. imgproxy 维护停滞或 license 收紧（当前为 [Apache-2.0](https://github.com/imgproxy/imgproxy)，releases 高频）→ 按本文件对比表重估，Imagor 优先。
  4. 团队决定不再自运维变换层 → 才考虑 Cloudflare Images（R2 协同最好），但须先补齐签名机制、主权、成本三项核查（见下节）。

## 未核实清单（诚实声明，不代结论）

- willnorris 与 Picfit 的 metrics.go 是否为 Prometheus exposition：仅确认 [文件存在](https://github.com/willnorris/imageproxy) / [文件存在](https://github.com/thoas/picfit)，未读源码。
- Cloudflare Images 与需求同义的签名/防盗刷机制：[既定路径 404](https://developers.cloudflare.com/images/optimization/security/)，未确认。
- Bunny 的签名/token 与 R2 回源细节：未核实。
- imgproxy 自官方 benchmark 的量化数字：README 仅链接 [benchmark 报告](https://github.com/imgproxy/imgproxy)（页内引用的存在性），本文件未抓取该报告、不转述其数字；性能引述只采用已抓取的 [imagor 官方 benchmark 页](https://docs.imagor.net/benchmarks) 并附方法。
- 各仓库 star / issue 数为 2026-09-14 抓取值，会漂移；[thumbor 7.8.0 的 CVE/GHSA 编号](https://github.com/thumbor/thumbor/releases)（官方称将另行发布 advisory，本文件不编造编号）。

## 一手来源索引

- imgproxy：[仓库](https://github.com/imgproxy/imgproxy) · [releases](https://github.com/imgproxy/imgproxy/releases) · [签名](https://docs.imgproxy.net/usage/signing_url) · [配置总览](https://docs.imgproxy.net/configuration/options) · [本地源](https://docs.imgproxy.net/image_sources/local_files) · [S3 源](https://docs.imgproxy.net/image_sources/amazon_s3) · [格式](https://docs.imgproxy.net/image_formats_support) · [Prometheus](https://docs.imgproxy.net/monitoring/prometheus)
- Imagor：[仓库](https://github.com/cshum/imagor) · [releases](https://github.com/cshum/imagor/releases) · [benchmark](https://docs.imagor.net/benchmarks) · [benchmark 源项目](https://github.com/cshum/image-servers-benchmark) · [安全](https://docs.imagor.net/security) · [配置](https://docs.imagor.net/configuration) · [S3/R2](https://docs.imagor.net/storage-s3) · [filters](https://docs.imagor.net/filters)
- Thumbor：[仓库](https://github.com/thumbor/thumbor) · [releases](https://github.com/thumbor/thumbor/releases) · [安全/签名](https://thumbor.readthedocs.io/en/latest/security.html)
- Picfit：[仓库（thoas/picfit）](https://github.com/thoas/picfit) · [releases](https://github.com/thoas/picfit/releases)
- willnorris/imageproxy：[仓库](https://github.com/willnorris/imageproxy) · [releases](https://github.com/willnorris/imageproxy/releases)
- Cloudflare Images：[总览](https://developers.cloudflare.com/images/) · [参数](https://developers.cloudflare.com/images/optimization/features/) · [定价](https://developers.cloudflare.com/images/pricing/)
- Bunny：[Dynamic Images](https://bunny.net/docs/optimizer/dynamic-images/overview) · [定价](https://docs.bunny.net/optimizer/pricing) · [limits](https://docs.bunny.net/optimizer/limits)
