# Public Site

Public Site 是 IdeaBoard 面向未登录访客、搜索引擎和 answer engine 的公开产品与内容上下文。它与认证后的 Canvas Workspace 分离，拥有唯一的 public canonical URL 体系。

## Language

**Public Site**:
IdeaBoard 无需登录即可访问、允许被发现和引用的品牌、产品与内容 surface。`https://ideaboard.art/` 是其 canonical root。
_Avoid_: Landing SPA, User Home, marketing shell

**Public Page**:
Public Site 中拥有稳定 canonical URL 的独立公开内容单元。
_Avoid_: Auth route, Workspace route, crawler-only page

**Workspace Route**:
`/user/*` 下承载登录、Boards、Canvas 和账户操作的应用 route，不承担搜索获客职责。
_Avoid_: Public Page, landing page

**Visual Designer**:
通过图像、构图、风格和视觉系统完成创意工作的独立设计师、品牌视觉设计师或小型创意团队，是 Public Site 的 primary audience。
_Avoid_: Everyone who needs AI images, generic AI user

**Pattern Designer**:
以图案概念和视觉重复为主要创作对象的 Visual Designer，是 Public Site 的首要专业细分人群。Pattern Ideation 是 flagship Use Case，但不代表当前产品覆盖完整的 surface-pattern production workflow。
_Avoid_: Generic image generator user, complete production-suite customer

**Workspace Activation**:
访客完成认证并首次进入可工作的 Canvas Board，是 Public Site 的 primary conversion。注册或登录只是 conversion funnel 中的中间步骤。
_Avoid_: Signup conversion, CTA click

**Use Case**:
Visual Designer 希望完成的端到端创作结果，是 Public Site 的一级内容主线。
_Avoid_: Workflow feature, tool list, capability

**Capability Page**:
解释一项真实可执行图像能力及其输入、输出、限制和适用场景的 Public Page，用于支撑相关 Use Case。
_Avoid_: Use Case, generic feature list

**Content Cluster**:
由一个旗舰 Use Case 与后续可独立发布的 Capability Page、指南组成的内容集合。首个 Content Cluster 聚焦 Pattern Ideation，首发不要求 demo 或输出示例。
_Avoid_: Thin feature-page collection, generated keyword pages

**Pattern Ideation**:
通过 Prompt、视觉方案比较、参考风格融合、局部精修与高清输出探索图案方向的创作过程，是当前 Pattern Design Content Cluster 的 product promise。
_Avoid_: Production-ready seamless repeat, print-production workflow

**Public Language**:
首发 Public Site 的 canonical content language 是 English。页面正文、metadata、导航和 CTA 统一使用 English；未来完整维护 Chinese 内容时使用独立 `/zh-CN/*` 页面并建立双向语言关系。
_Avoid_: Mixed-language page, partial translation, machine-translated alternate

**AI Canvas**:
IdeaBoard 面向 Visual Designer 的产品类别；通过 connected image workflows 支持视觉想法的探索、组合与迭代。
_Avoid_: Generic AI image generator, prompt-only tool

**Search Citation Access**:
允许合规的 search 与 answer engine crawler 发现、理解和引用 Public Site 内容的访问许可；它不等于训练许可。
_Avoid_: Training access, user-request access

**Training Crawl Access**:
允许内容进入模型训练或改进数据收集的独立访问许可，由产品与内容权利策略单独决定。
_Avoid_: Search indexing, citation access

**Public Rendering Boundary**:
Public Site routes 以 build-time static HTML 交付，Workspace、Admin 和受保护的应用 routes 继续由 Vue SPA/CSR 交付；两者共享产品事实与品牌资产，但不共享 rendering responsibility。
_Avoid_: One rendering mode for every route, bot-specific rendering

**Public Fact Registry**:
由代码拥有、可被 executable workflow contract 验证的 Public Site 稳定事实集合，例如 capability slug、名称、输入、输出、限制、公开状态和 Use Case 关联。
_Avoid_: Admin Prompt source, runtime-discovered claim, duplicated metadata

**Editorial Content**:
经过 review、面向 Visual Designer 的 English 页面叙事、步骤和设计语境；它引用 Public Fact Registry，但不重新定义能力事实或虚构示例证据。
_Avoid_: Generated SEO copy, hidden crawler text, prompt template

**Repository-reviewed Publishing**:
Editorial Content、Public Fact Registry、metadata、structured data 和 sitemap 通过同一 repo 的 review 与 build 一起发布，而不是由运行时 Admin CMS 修改。
_Avoid_: Runtime SEO CMS, prompt-driven publishing, unreviewed copy

**Canonical URL Contract**:
Public Page 使用 `https://ideaboard.art`、lowercase `kebab-case`、无 trailing slash 的稳定路径；内容不通过 query 或 hash 表达，非 canonical 变体统一重定向。
_Avoid_: Query-shaped content URL, duplicate locale root, trailing-slash variants

**Content Provenance**:
Public Content 默认由 IdeaBoard 或 IdeaBoard Editorial Team 署名，并提供真实 Published/Last reviewed 日期，不虚构个人作者、评价或资质。
_Avoid_: Synthetic authority, anonymous AI copy, fabricated review

**Structured Data Contract**:
JSON-LD 只表达页面可见且可验证的事实；首页使用 `Organization`/`WebSite`，导航层使用 `BreadcrumbList`，真实指南才使用 `Article`/`TechArticle`，产品事实充分时才使用 `SoftwareApplication`。
_Avoid_: Hidden claims, fabricated rating, commercial FAQ rich-result bait

**Public Pricing**:
由 Public Fact Registry 拥有、可公开验证且有明确维护者的 pricing facts；首发 Content Cluster 不公开 Pricing，也不在 structured data 中声明 `offers`。
_Avoid_: Admin-only price, stale offer, implied free pricing

**First-party Organic Measurement**:
使用 Search Console、Bing Webmaster Tools、Cloudflare/source logs 和 server-side Workspace Activation event 衡量 Public Site 的发现、抓取和转化，不依赖 third-party tracking cookies 或用户内容分析。
_Avoid_: Cross-site profile, session replay, prompt/content surveillance

**Authenticated App Surface**:
承载登录、账户、Boards、Canvas、Admin 和 tokenized share 操作的受保护 route 集合；它们不属于 Public Site，不进入 sitemap，并交付 `noindex`。
_Avoid_: Public Page, searchable workspace, robots-only privacy

**Public URL Integrity**:
只有已发布且存在的 Public Page 返回 `200`；不存在、撤销或迁移的 URL 分别返回真实 `404`、`410` 或明确的 `301`，不得用统一 SPA shell 伪装成功页面。
_Avoid_: Soft 404, unknown-route 200, blanket SPA fallback

**Public Content Lifecycle**:
Public Content 只有 `draft`、`published` 和 `retired` 三个状态；build 只发布 `published` 页面，`retired` 页面按替代关系投影为 `301` 或 `410`。
_Avoid_: Runtime publish state, scheduled CMS workflow, stale sitemap entry

**Initial Product URL Set**:
首发只发布 `/` 和 `/use-cases/pattern-ideation`。Pattern Generation、Style Variation、Style Fusion、Partial Editing 和 Image Upscaling 的 Capability Page 保持 `draft`，在具备可信 supporting evidence 前不生成 URL。
_Avoid_: Premature feature sprawl, thin capability page, unverified route

**Public Page Content Contract**:
每个 published Public Page 必须有 opening answer、target Visual Designer、真实 workflow steps、输入输出与限制、相关内部链接、Workspace Activation CTA 及 Content Provenance，并与 metadata/JSON-LD 保持一致。
_Avoid_: Metadata-only page, unsupported claim, fabricated evidence

**Content Cluster Link Graph**:
Pattern Ideation Use Case 是首发 Content Cluster 的 hub；首发首页只链接该 hub。Capability Page 发布后再链回 hub，并按 workflow 顺序链接相邻能力，不链接尚未发布的 route。
_Avoid_: Orphan page, indiscriminate cross-linking, vague anchor text

**Anonymous Public Content**:
Public Page 的正文、workflow steps、输入输出、限制和 metadata 无需登录即可读取；只有 Workspace Activation CTA 进入认证流程。
_Avoid_: Auth-gated explanation, client-only SEO body, user asset preview

**Intent-Preserving Handoff**:
Public Page CTA 进入 Workspace 时携带对应 Use Case 或 Capability intent；intent 只影响应用内 start flow，不改变 Public Page canonical URL 或 sitemap。
_Avoid_: Query-shaped canonical, lost use-case context, forced overwrite
