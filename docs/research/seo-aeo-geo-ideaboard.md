# IdeaBoard SEO / AEO / GEO 基线研究

> 范围：`ideaboard.art` 的 Vue 3 + Vite SPA、Cloudflare Pages，以及调研开始时位于 `/user/home` 的公开 Landing。本文把**厂商明确说明**、**基于说明的工程推论**和**证据不足**分开；外部事实只引用一手资料。调研日期：2026-08-12。

## 结论与优先级

| 优先级 | 必须做什么                                                                                                                               | 依据与目标                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| ------ | ---------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| P0     | 将可检索的营销/产品内容做成独立的静态 HTML（SSG / prerender）；`/` 应直接返回该 Landing 的 `200` HTML，而非依赖 SPA client redirect。    | **厂商明确建议**：Google 把 `dynamic rendering` 定位为 workaround，并建议 SSR、static rendering 或 hydration；Google 能处理 JavaScript，但公开内容不应以此作为唯一交付路径。[Google JavaScript SEO](https://developers.google.com/search/docs/crawling-indexing/javascript/javascript-seo-basics) [Google rendering guidance](https://developers.google.com/search/docs/crawling-indexing/javascript/dynamic-rendering)                                                                                                                                                                                                                                        |
| P0     | 重做 `robots.txt`、`sitemap.xml` 和 `noindex` 策略；只提交真实、公开、canonical、可返回 `200` 的 URL。                                   | **厂商明确说明**：sitemap 仅是提示而非索引保证；`robots.txt` 管理 crawl，不是从搜索中隐藏 URL 的手段。[Google sitemap overview](https://developers.google.com/search/docs/crawling-indexing/sitemaps/overview) [Google robots guide](https://developers.google.com/search/docs/crawling-indexing/robots/intro)                                                                                                                                                                                                                                                                                                                                                 |
| P0     | 给公开页交付独立的 title、description、canonical、Open Graph 和语言；给登录、账户、Canvas、Admin、分享令牌页交付 HTTP 可见的 `noindex`。 | **厂商明确说明**：Google 支持 `<meta name="robots">` 与 `X-Robots-Tag` 的 `noindex`；canonical 是首选 URL 的强信号之一。[Google robots meta](https://developers.google.com/search/docs/crawling-indexing/robots-meta-tag) [Google canonicalization](https://developers.google.com/search/docs/crawling-indexing/consolidate-duplicate-urls)                                                                                                                                                                                                                                                                                                                    |
| P1     | 围绕真实用户问题建立可被引用的公开内容页，并用 Product/Organization 事实支撑，而不是为 AI 另造隐藏文案。                                 | **厂商明确说明**：Google AI Overviews / AI Mode 没有额外技术要求或专用 schema；页面必须先能被索引，既有 SEO 基础仍适用。[Google AI features](https://developers.google.com/search/docs/appearance/ai-features)                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| P1     | 通过 Search Console、Bing Webmaster Tools、Rich Results Test、部署后的 HTTP 响应和已验证 crawler 日志闭环。                              | **厂商明确说明**：Google 提供 URL Inspection、Crawl Stats 和 Rich Results Test；OpenAI/Anthropic/Perplexity 公布 bot IP 来源以供验证。[Google URL Inspection](https://support.google.com/webmasters/answer/9012289) [Google Crawl Stats](https://support.google.com/webmasters/answer/9679690) [Rich Results Test](https://search.google.com/test/rich-results) [OpenAI bots](https://platform.openai.com/docs/bots) [Anthropic crawler](https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler) [Perplexity crawlers](https://docs.perplexity.ai/docs/resources/perplexity-crawlers) |
| P2     | 经产品确认后设置 AI crawler 的 allow/block policy；不把 `llms.txt` 当作收录、训练或访问控制。                                            | **厂商明确说明 / 证据不足**：各厂商把各自 bot 的 `robots.txt` 规则作为控制面；`llms.txt` 仓库本身只是提案，未见 Google、Bing、OpenAI 或 Anthropic 在其 crawler 文档中声明支持它。[OpenAI bots](https://platform.openai.com/docs/bots) [Google crawler overview](https://developers.google.com/search/docs/crawling-indexing/overview-google-crawlers) [Anthropic crawler](https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler) [llms.txt proposal](https://github.com/AnswerDotAI/llms-txt)                                                                                        |

## 实施前仓库审计（研究快照）

以下为仓库事实，不是外部规则：

- [`apps/web/index.html`](../../apps/web/index.html) 只有全局 `Idea Board` title；没有 description、canonical、Open Graph、robots meta 或 JSON-LD。所有 SPA route 初始都依赖同一份 HTML。
- [`router/index.ts`](../../apps/web/src/app/router/index.ts) 使用 `createWebHistory()`，且 `/` client redirect 到 `/user/home`；[`userRoutes.ts`](../../apps/web/src/features/user/app/userRoutes.ts) 显示 `/user/home`、`/user/login`、`/user/register`，并将 `/user/boards`、`/user/canvas` 标记为 `requiresAuth`。
- 未跟踪的 [`apps/web/public/sitemap.xml`](../../apps/web/public/sitemap.xml) 列出 `/boards`、`/login`、`/register`，但它们并非当前 router URL；它还把 Login/Register 放入 sitemap。当前真实认证路径 `/user/boards`、`/user/canvas` 不应出现在 sitemap。
- 未跟踪的 [`apps/web/public/robots.txt`](../../apps/web/public/robots.txt) 已排除 `/admin/`、`/api/`、`/auth/`、`/canvas/`、`/share/`，但实际私有应用路径是 `/user/boards` 与 `/user/canvas`，因此覆盖不完整。它没有针对 `Googlebot`、`Bingbot` 或 AI bot 的策略。
- sitemap 中的 `lastmod=2026-08-12`、`changefreq` 和 `priority` 不应例行伪造或用作排序信号：Google 说明 `lastmod` 只应在页面有显著更新时使用，且会忽略 `priority` 与 `changefreq`。[Google build sitemap](https://developers.google.com/search/docs/crawling-indexing/sitemaps/build-sitemap)

## 建议的 Information Architecture

**推荐架构（工程推论）**：把搜索面从认证应用中分离。在同一 Cloudflare Pages 项目中，先在 build 时生成 `/`、`/features/*`、`/use-cases/*`、`/pricing`、`/templates/*`、`/docs/*`、`/terms`、`/privacy-policy` 等公开静态 HTML；继续让 `/user/*`、`/admin/*`、`/share` 运行现有 SPA。Cloudflare Pages 支持以 `_headers` 和 `_redirects` 配置静态资产响应，适合把公开页的 canonical/robots header 与应用路由区分部署。[Cloudflare Pages headers](https://developers.cloudflare.com/pages/configuration/headers/) [Cloudflare Pages redirects](https://developers.cloudflare.com/pages/configuration/redirects/)

| URL 面                                                                                                                 | 渲染与索引策略                                                                                                                                          | 理由                                                                                                                                                                                                                                                                                                          |
| ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `/`、产品能力/定价/教程/模板详情等无登录公开页                                                                         | SSG / prerender，初始 HTML 中有正文、标题、metadata、canonical 和可验证 JSON-LD；`index,follow`。                                                       | **厂商明确建议 + 工程推论**：Google 推荐 static rendering / SSR 处理 JavaScript 公开内容；预渲染减少对 JS 执行成功、时序和 crawler 差异的依赖。[Google rendering guidance](https://developers.google.com/search/docs/crawling-indexing/javascript/dynamic-rendering)                                          |
| `/terms`、`/privacy-policy`、`/refund-policy`、`/acceptable-use`                                                       | 可索引的静态 HTML，是否进入 sitemap 取决于产品是否希望它们被发现；保持可访问、正确 canonical。                                                          | **厂商明确说明**：sitemap 应包含希望 Google 收录的 canonical URL；不是站内 URL 的全量清单。[Google sitemap overview](https://developers.google.com/search/docs/crawling-indexing/sitemaps/overview)                                                                                                           |
| `/user/login`、`/user/register`、`/user/auth/callback`、`/user/boards`、`/user/canvas`、`/admin/*`、`/share?token=...` | 不进 sitemap；返回 `X-Robots-Tag: noindex, nofollow` 或 HTML `<meta name="robots" content="noindex,nofollow">`。认证与授权仍由后端执行，不能靠 robots。 | **厂商明确说明**：`noindex` 是阻止收录的机制，`robots.txt` 不是安全控制且被 robots 阻止的 URL 仍可能被索引。[Google robots guide](https://developers.google.com/search/docs/crawling-indexing/robots/intro) [Google robots meta](https://developers.google.com/search/docs/crawling-indexing/robots-meta-tag) |
| 不存在、被撤销分享链接、永久删除的公开资源                                                                             | 返回真实 `404` / `410`，不要把任意未知 path 都以 `200` SPA shell 成功响应。                                                                             | **厂商明确说明**：Google 的 JavaScript 指南要求有意义的 HTTP status；错误页须以正确状态码交付。[Google JavaScript SEO](https://developers.google.com/search/docs/crawling-indexing/javascript/javascript-seo-basics)                                                                                          |

不要为 crawler 生成与用户不同的营销正文。若采用 bot 识别做 rendering，页面内容必须相近，否则可能被视为 cloaking；更好的路径是同一份静态 HTML 后 hydration。**厂商明确说明**：[Google dynamic rendering and cloaking](https://developers.google.com/search/docs/crawling-indexing/javascript/dynamic-rendering)

## Crawl、Index 与技术元数据

### 公开页交付契约

每个可索引 URL 应在**初始 HTML**中拥有以下内容；这是基于 Google 对可抓取、可索引 HTML 和 canonical 的说明作出的工程建议，不是要求使用某个 Vue library。

1. 唯一且描述性的 `<title>`、简洁的 `<meta name="description">`、一个 `<h1>`，以及能回答该页主题的可见正文；Google 的 SEO Starter Guide 建议使用描述性 title、meta description 和清晰的标题层级。[Google SEO Starter Guide](https://developers.google.com/search/docs/fundamentals/seo-starter-guide)
2. 一个绝对 URL 的 `<link rel="canonical" href="https://ideaboard.art/...">`；所有 HTTP/HTTPS、末尾斜杠、query 参数和重复 Landing 变体都 `301` 到它。Google 将 `rel="canonical"` 视为 canonicalization 的强信号，但仍会综合 sitemap、内部链接和其他信号。[Google canonicalization](https://developers.google.com/search/docs/crawling-indexing/consolidate-duplicate-urls)
3. 与 canonical 一致的 `og:title`、`og:description`、`og:url` 和绝对 `og:image`；Open Graph 用于社交图谱/预览协议，不是 Google 收录要求。**厂商明确说明**：[Open Graph protocol](https://ogp.me/)
4. 主语言使用 `<html lang="zh-CN">`；未来有完整、可互相对应的英文页时再增加 `hreflang`。Google 要求 alternate language 页面本身可被找到并使用双向 `hreflang` 关系；不要把机器翻译或只改 URL 的空壳标为替代语言。[Google localized versions](https://developers.google.com/search/docs/specialty/international/localized-versions)

### `robots.txt`、`noindex` 与 sitemap

**官方规则**：`robots.txt` 是根路径的 crawler 访问规则；其行为由 [RFC 9309](https://www.rfc-editor.org/rfc/rfc9309.html) 定义，而 Google 强调它不能隐藏页面，也不能强制不守规则的 crawler。[Google robots guide](https://developers.google.com/search/docs/crawling-indexing/robots/intro)

**对 IdeaBoard 的建议**：

- 新 sitemap 只保留经产品确认的静态公开 canonical URL；删除 `/boards`、`/login`、`/register` 与一切私有、带 token、带 query 的 URL。每个 `<loc>` 必须与最终 canonical 完全相同，`lastmod` 只在内容实质变更时更新。[Google build sitemap](https://developers.google.com/search/docs/crawling-indexing/sitemaps/build-sitemap)
- `robots.txt` 保留 `Sitemap: https://ideaboard.art/sitemap.xml`，并用它减少无价值资源/API 的 crawl；不要把 `Disallow: /user/` 当作 `noindex` 的替代。若私有 URL 过去已被外链发现，应先可抓取地交付 `noindex`，待 Search Console 确认移除后再评估是否需要 `Disallow`。这是直接由 Google 对“robots 阻止后仍可能索引”的警告推导出的实施顺序。[Google robots guide](https://developers.google.com/search/docs/crawling-indexing/robots/intro)
- 静态资源不要误封：Google 需要获取 JavaScript、CSS、图片等资源来正确渲染页面。[Google JavaScript SEO](https://developers.google.com/search/docs/crawling-indexing/javascript/javascript-seo-basics)
- 用 Cloudflare Pages 的 `_headers` 对受控路径交付 `X-Robots-Tag`，并在预览与生产环境以 `curl -I` 验证；Cloudflare 说明 `_headers` 可为静态资产配置 HTTP response headers。[Cloudflare Pages headers](https://developers.cloudflare.com/pages/configuration/headers/)

## Structured Data

Structured data 必须与页面中用户能看见的真实内容一致、遵守对应 feature 的 required properties，且不保证 rich result。**厂商明确说明**：[Google structured data introduction](https://developers.google.com/search/docs/appearance/structured-data/intro-structured-data) [Google structured data policies](https://developers.google.com/search/docs/appearance/structured-data/sd-policies)

| 页面                           | 推荐类型                                                                                                   | 边界                                                                                                                                                                                                                                                                                                                            |
| ------------------------------ | ---------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 首页/全站                      | `Organization`、`WebSite`；可用稳定品牌名、URL、logo、sameAs（真实官方账号）。                             | `WebSite`/`Organization` 词汇由 Schema.org 定义；Google 的站点名功能以 `WebSite` markup 建议站点名。不要虚构 `sameAs` 或 logo。[Schema.org Organization](https://schema.org/Organization) [Schema.org WebSite](https://schema.org/WebSite) [Google site names](https://developers.google.com/search/docs/appearance/site-names) |
| 真实产品 Landing               | `SoftwareApplication`，只填写页面公开可验证的 application category、operating system、offers、价格与评分。 | Google 有 Software App rich result 文档；没有公开 price/评价就不要填 offer/rating，也不要制造 aggregateRating。[Google Software App](https://developers.google.com/search/docs/appearance/structured-data/software-app) [Schema.org SoftwareApplication](https://schema.org/SoftwareApplication)                                |
| 可公开索引的教程/文档          | `Article` 或 `TechArticle`，标明作者/发布日期/更新时间与正文一致。                                         | 仅在内容确为文章且属性可由页面验证时使用；Google 要求 structured data 不误导且内容可访问。[Google structured data policies](https://developers.google.com/search/docs/appearance/structured-data/sd-policies)                                                                                                                   |
| 可检索的公开模板集合及模板详情 | `BreadcrumbList`，必要时模板详情使用反映真实内容的类型。                                                   | breadcrumb 必须反映用户可导航的层级，不能伪造搜索路径。[Google breadcrumb markup](https://developers.google.com/search/docs/appearance/structured-data/breadcrumb)                                                                                                                                                              |

不要为商业 FAQ 期待 FAQ rich result。Google 目前把 FAQ rich result 限于政府或健康领域的权威站点；仍可发布有用 FAQ 正文，但 markup 不会带来该展示。**厂商明确说明**：[Google FAQ structured data](https://developers.google.com/search/docs/appearance/structured-data/faqpage)

## AEO / GEO：可验证的内容策略

这里的 AEO/GEO 指让公开内容更容易被传统搜索、AI Overviews、AI Mode 和外部 answer engine 找到、理解与引用，**不**指承诺某个回答会展示。Google 明确表示 AI features 没有额外 requirements、没有专用 schema，且 Supporting Link 的前提是页面先被索引。[Google AI features](https://developers.google.com/search/docs/appearance/ai-features)

**厂商明确建议**：优先 people-first、原创、有第一手经验、清楚展示“谁创建、如何创建、为什么创建”的内容；不要为搜索流量批量生产无附加价值的页面。[Google helpful content](https://developers.google.com/search/docs/fundamentals/creating-helpful-content)

**由上述规则导出的内容设计推论**：

- 为 IdeaBoard 的真实工作流建独立页面，例如“电商主图从草图到成图”“多图风格融合”“局部重绘”“背景移除”“无缝图案生成”“图片放大”。每页开头以一两句直接回答核心问题，随后给出步骤、限制、输入/输出示例、可复现 prompt 范例和原创示例图。
- 公开模板页要说明适用场景、可编辑参数、版权/素材限制和实际效果；不能把需要登录或不存在的模板 URL 放进 sitemap。答案、图片、表格和事实必须在用户可见 HTML 中，而非只放 JSON-LD、注释或 bot-only DOM。
- 每项能力都应有一个稳定的 canonical URL，标题采用用户会问的具体任务而非只有品牌口号；链接使用描述性 anchor text。该做法服务于发现与理解，符合 Google 对清晰内容组织和描述性链接的建议。[Google SEO Starter Guide](https://developers.google.com/search/docs/fundamentals/seo-starter-guide)
- 对价格、生成能力、限制、支持模型和数据处理只陈述可验证的当前事实；变化时更新页面与 `lastmod`。这降低错误引用风险，亦符合 Google 对可靠、透明内容的要求。[Google helpful content](https://developers.google.com/search/docs/fundamentals/creating-helpful-content)

内容预览控制是取舍，而非 GEO 优化按钮：`nosnippet`、`data-nosnippet` 与 `max-snippet` 可限制 Google 的 snippet/AI feature 使用的文本，但也会减少可展示/可引用的材料。仅对确实不应被摘录的敏感段落使用。**厂商明确说明**：[Google robots meta and snippet controls](https://developers.google.com/search/docs/crawling-indexing/robots-meta-tag) [Google AI features](https://developers.google.com/search/docs/appearance/ai-features)

## AI crawler 与生成式搜索控制

`robots.txt` 是对遵守规则的 crawler 的声明，不能替代访问控制、鉴权、速率限制或版权/合同条款；高价值私有 Canvas、分享 token 和上传资产必须仍由现有授权边界保护。[RFC 9309](https://www.rfc-editor.org/rfc/rfc9309.html) [Google robots limitations](https://developers.google.com/search/docs/crawling-indexing/robots/intro)

| 厂商/目标                           | 可用 User-agent 与建议                                                                                                                                                     | 状态                                                                                                                                                                   |
| ----------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Google Search 与 Google AI features | 保持 `Googlebot` 可访问公开页。Google AI Overviews/AI Mode 没有另一个 allow-list bot 或专用 markup；收录是前提。                                                           | **厂商明确说明**：[Google AI features](https://developers.google.com/search/docs/appearance/ai-features)                                                               |
| Google 训练/grounding 选择          | 若产品决定不让公开内容帮助改进 Gemini Apps / Vertex AI generative APIs，可加 `User-agent: Google-Extended` / `Disallow: /`；这不影响 Google Search 排名。                  | **厂商明确说明**：[Google crawler overview](https://developers.google.com/search/docs/crawling-indexing/overview-google-crawlers)                                      |
| OpenAI Search                       | 若希望在 ChatGPT Search 被发现，明确允许 `OAI-SearchBot`；OpenAI 说明它用于 ChatGPT Search，且与 GPTBot 的设置独立。                                                       | **厂商明确说明**：[OpenAI bots](https://platform.openai.com/docs/bots)                                                                                                 |
| OpenAI 模型训练                     | 若选择退出未来公开内容的模型训练，单独加 `User-agent: GPTBot` / `Disallow: /`，同时可继续允许 `OAI-SearchBot`。                                                            | **厂商明确说明**：[OpenAI bots](https://platform.openai.com/docs/bots)                                                                                                 |
| OpenAI 用户主动访问                 | `ChatGPT-User` 是用户触发而非自动 web crawl；OpenAI 说明 `robots.txt` 规则可能不适用，不能把它视为可靠的屏蔽方法。                                                         | **厂商明确说明**：[OpenAI bots](https://platform.openai.com/docs/bots)                                                                                                 |
| Microsoft / Bing                    | 管理公开搜索发现用 `Bingbot` 与 Bing Webmaster Tools；本次一手文档未找到 Microsoft 声明、可替代 `Google-Extended`/`GPTBot` 的独立 AI training user-agent，故不能臆造规则。 | **证据不足**；仅采用其公开 crawler/tool 说明：[Bing crawler guidance](https://www.bing.com/webmasters/help/which-crawlers-does-bing-use-8c184ec0)                      |
| Anthropic                           | `ClaudeBot` 控制可能进入模型训练的自动 crawl；若退出，使用 `User-agent: ClaudeBot` / `Disallow: /`。其文档另列 `Claude-User` 等用途，需按是否允许用户请求访问分别确认。    | **厂商明确说明**：[Anthropic crawler](https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler) |
| Perplexity（可选）                  | `PerplexityBot` 用于其 search result 引用而非 foundation-model training；是否允许应按想不想在 Perplexity 被发现决定。用户请求的 `Perplexity-User` 一般忽略 robots。        | **厂商明确说明**：[Perplexity crawlers](https://docs.perplexity.ai/docs/resources/perplexity-crawlers)                                                                 |

产品确认“可被搜索引用”但“不可用于训练”时，最小草案应类似下列片段；上线前应与法务/内容权利决策一起核对，而不是照抄：

```text
User-agent: OAI-SearchBot
Allow: /

User-agent: GPTBot
Disallow: /

User-agent: Google-Extended
Disallow: /

User-agent: ClaudeBot
Disallow: /
```

以上不会阻止 Google Search 或 Bing Search，也不会让私有 URL 安全；各 bot 的真实性须用其公布 IP/验证流程和 Cloudflare 日志核验，不能仅信任可伪造的 `User-Agent`。[Googlebot verification](https://developers.google.com/search/docs/crawling-indexing/verifying-googlebot) [OpenAI bots](https://platform.openai.com/docs/bots) [Anthropic crawler](https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler)

### Cloudflare Content Signals 与 Markdown negotiation

Cloudflare AI Crawl Control 的 Directives 会检测 `robots.txt` 是否包含 Content Signals；当前产品策略为 `ai-train=no, search=yes, ai-input=yes`，允许公开内容用于 search、RAG、grounding 和实时生成式回答，但拒绝用于模型训练。该语法来自 Content Signals / AIPREF 提案，不属于 RFC 9309 的标准 robots 指令，因此它是 crawler policy 的补充声明，不替代 `Allow`、`Disallow`、`noindex` 或 authorization。[Cloudflare AI Crawl Control directives](https://developers.cloudflare.com/ai-crawl-control/features/track-robots-txt/) [Content Signals](https://contentsignals.org/) [Expired AIPREF draft](https://datatracker.ietf.org/doc/draft-romm-aipref-contentsignals/)

Cloudflare Markdown for Agents 在 Zone 或 Configuration Rule 启用后，会根据 `Accept: text/markdown` 在 edge 将原始 HTML 转换为 Markdown，并自动设置 `Content-Type: text/markdown`、`Vary: Accept`、`x-markdown-tokens` 和 `x-original-tokens`。它会保留源站的 `Content-Signal` header；若源站没有该 header，Cloudflare 默认声明 `ai-train=yes, search=yes, ai-input=yes`。因此代码侧应给公开 HTML 设置与 robots policy 一致的 `Content-Signal` header，Markdown negotiation 则在 Cloudflare Dashboard 开启，不应在 Vue SPA 中重复实现 converter。[Cloudflare Markdown for Agents](https://developers.cloudflare.com/fundamentals/reference/markdown-for-agents/)

线上验证必须直接请求生产边界：`/robots.txt` 和 `/sitemap.xml` 应返回 `200` 且分别为 `text/plain` 与 XML；公开页面带有 `Content-Signal` header；对 `Accept: text/markdown` 的请求应返回 Markdown。若这些请求返回 SPA `index.html`，说明静态产物尚未部署或 Pages fallback 覆盖了静态文件，不能用本地文件存在替代生产验证。

### `llms.txt` 的证据边界

`llms.txt` 的维护者将其定义为“为 LLM-friendly content 提供 Markdown 文件”的提案格式。它**不是** IETF/W3C robots 标准，也不是 Google、Bing、OpenAI 或 Anthropic 在上述官方 crawler 文档承诺解析的指令。因此：

- 可在公开文档体系成熟后，作为面向人和工具的补充导航文件发布，且只链接真正公开、canonical、可读的内容；这是**可选实验**。[llms.txt proposal](https://github.com/AnswerDotAI/llms-txt)
- 不得用它替代 sitemap、`robots.txt`、`noindex`、access control 或训练退出；这些职责分别依赖 sitemap、RFC robots 规则、robots meta/header 和各厂商 bot policy。**证据不足以支持任何更强保证**。[RFC 9309](https://www.rfc-editor.org/rfc/rfc9309.html) [OpenAI bots](https://platform.openai.com/docs/bots) [Google crawler overview](https://developers.google.com/search/docs/crawling-indexing/overview-google-crawlers)

## 验证与度量闭环

1. 部署前：对每个公开 canonical URL 检查 `200`、首字节 HTML 含正文/title/description/canonical/JSON-LD；对私有 URL 检查鉴权和 `X-Robots-Tag: noindex`；未知 URL 检查真实 `404`。这是对 Google JavaScript/HTTP status 与 robots meta 要求的直接验证。[Google JavaScript SEO](https://developers.google.com/search/docs/crawling-indexing/javascript/javascript-seo-basics) [Google robots meta](https://developers.google.com/search/docs/crawling-indexing/robots-meta-tag)
2. 发布后：在 Google Search Console 验证 domain property，提交 sitemap，用 URL Inspection 检查 canonical、抓取和 rendered HTML，用 Crawl Stats 找失败/资源阻塞；使用 Rich Results Test 验证合格的 structured data。**厂商明确说明**：[Google URL Inspection](https://support.google.com/webmasters/answer/9012289) [Google Crawl Stats](https://support.google.com/webmasters/answer/9679690) [Rich Results Test](https://search.google.com/test/rich-results)
3. 同步在 Bing Webmaster Tools 验证站点、提交 sitemap，使用其 URL Inspection、robots tester 与 index coverage 报告检查 Bingbot 可访问性。**厂商明确说明**：[Bing crawler guidance](https://www.bing.com/webmasters/help/which-crawlers-does-bing-use-8c184ec0)
4. 每月按 landing/content cluster 观察 Search Console 的 query、impression、click、CTR、indexed URL 和 conversion；Google 将 AI feature 流量包含在整体 Search Console 数据中，不能把一个单独的“AI answer 排名”当 KPI。**厂商明确说明**：[Google AI features measurement](https://developers.google.com/search/docs/appearance/ai-features)
5. 审计 Cloudflare/源站日志中的已验证 Googlebot、Bingbot 和已选择允许的 AI bot：记录 URL、status、响应大小、render asset 失败率和 robots 变更后结果。对未验证 UA 不放宽 WAF。Google 要求按公开 DNS/IP 流程验证 Googlebot；OpenAI、Anthropic、Perplexity 均公布其 IP 信息/说明。[Googlebot verification](https://developers.google.com/search/docs/crawling-indexing/verifying-googlebot) [OpenAI bots](https://platform.openai.com/docs/bots) [Anthropic crawler](https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler) [Perplexity crawlers](https://docs.perplexity.ai/docs/resources/perplexity-crawlers)

## 反模式

- 把 Login、Register、Canvas、Admin、分享 token 或不存在 route 加入 sitemap，或让所有 path 返回相同的 `200` SPA shell。
- 用 `robots.txt` 保护私有数据、实现 `noindex`，或先 `Disallow` 再期待 bot 读取页面上的 `noindex`。[Google robots guide](https://developers.google.com/search/docs/crawling-indexing/robots/intro)
- 只修改运行后 DOM 的 title/canonical/正文，初始 HTML 仍为空壳；或为 bot 交付不同正文。Google 推荐 static/SSR/hydration，而不推荐长期 dynamic rendering；不同内容可构成 cloaking。[Google dynamic rendering](https://developers.google.com/search/docs/crawling-indexing/javascript/dynamic-rendering)
- 对所有 URL 使用相同 title/description/canonical，伪造 `lastmod`、rating、price、review 或隐藏 JSON-LD。Google 要求 structured data 与可见内容一致、不得误导，且不会保证 rich result。[Google structured data policies](https://developers.google.com/search/docs/appearance/structured-data/sd-policies)
- 批量生成无第一手价值的“AI SEO”页面、为 FAQ markup 预期普遍 rich result、阻止 snippet 后又要求被 AI 引用。[Google helpful content](https://developers.google.com/search/docs/fundamentals/creating-helpful-content) [Google FAQ](https://developers.google.com/search/docs/appearance/structured-data/faqpage) [Google AI features](https://developers.google.com/search/docs/appearance/ai-features)
- 把 `llms.txt` 或单个可伪造 User-Agent 当作访问/训练控制的安全边界。[RFC 9309](https://www.rfc-editor.org/rfc/rfc9309.html) [llms.txt proposal](https://github.com/AnswerDotAI/llms-txt)

## 实施前需要产品确认

1. `ideaboard.art` 是否把 `/` 定义为公开品牌/产品 Landing，并把 `/user/*` 明确为认证应用？若否，应先确定唯一 public canonical root，避免两个 Landing 竞争。
2. 首批可索引页面名单是什么：仅产品页与法律页，还是包含 pricing、feature、use case、template、docs？公开模板/示例是否已取得图片、prompt、用户内容和商标的发布权？
3. 公开内容的语言策略是仅 `zh-CN`，还是同时维护 `en`？双语只应在两边都有完整等价内容后上线。[Google localized versions](https://developers.google.com/search/docs/specialty/international/localized-versions)
4. 公司对“被 ChatGPT/Perplexity/Claude 搜索引用”与“用于模型训练/grounding”的许可矩阵是什么？该决定决定 `OAI-SearchBot`、`GPTBot`、`Google-Extended`、`ClaudeBot` 的最终规则。
5. 哪些价格、能力、Provider/model 信息可公开且由谁维护？没有稳定事实源的内容不应做 structured data 或长期 sitemap URL。
