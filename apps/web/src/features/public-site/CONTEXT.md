# Public Site Context
Public Site owns the public product and content surface for unauthenticated visitors, search engines, and answer engines, with a single public canonical URL system.
It does NOT own Workspace routes, the authenticated app surface, Admin, or Canvas.

## Domain Language
| Term | Meaning |
| ---- | ------- |
| Public Site | Brand, product, and content surface accessible without login and open to discovery and citation; canonical root is `https://ideaboard.art/`. |
| Public Page | Standalone public content unit with a stable canonical URL. |
| Workspace Route | App route under `/user/*` for login, Boards, Canvas, and account actions; carries no search-acquisition duty. |
| Visual Designer | Independent designer, brand visual designer, or small creative team working through image, composition, style, and visual systems; the primary audience. |
| Pattern Designer | Visual Designer focused on pattern concepts and visual repetition; the first professional sub-segment. |
| Workspace Activation | Visitor completes auth and first enters a working Canvas Board; the primary conversion (signup/login are only mid-funnel steps). |
| Use Case | End-to-end creative outcome a Visual Designer wants; the first-level content thread. |
| Capability Page | Public Page explaining one real executable image capability: inputs, outputs, limits, and fit; supports a Use Case. |
| Content Cluster | One flagship Use Case plus independently publishable Capability Pages and guides; the first cluster targets Pattern Ideation. |
| Pattern Ideation | Exploring pattern directions via prompts, visual-option comparison, reference-style fusion, local refinement, and HD export; the current product promise. |
| Public Language | Canonical content language is English for body, metadata, nav, and CTAs; full Chinese ships later as separate `/zh-CN/*` pages with bidirectional links. |
| AI Canvas | IdeaBoard product category for Visual Designers; connected image workflows to explore, combine, and iterate visual ideas. |
| Search Citation Access | Permission for compliant search/answer-engine crawlers to discover, understand, and cite content; not a training license. |
| Training Crawl Access | Separate permission for model training or improvement data collection; decided by product and rights policy. |
| Public Rendering Boundary | Public Site routes ship as build-time static HTML; Workspace, Admin, and protected routes stay Vue SPA/CSR; they share facts and brand assets, not rendering responsibility. |
| Public Fact Registry | Code-owned stable facts verifiable by executable workflow contracts: capability slug, name, inputs, outputs, limits, public status, Use Case links. |
| Editorial Content | Reviewed English narrative, steps, and design context for Visual Designers; cites the registry without redefining facts or inventing evidence. |
| Repository-reviewed Publishing | Editorial Content, registry, metadata, structured data, and sitemap ship together through repo review and build, never via a runtime Admin CMS. |
| Canonical URL Contract | Stable paths on `https://ideaboard.art`, lowercase `kebab-case`, no trailing slash; no content in query or hash; non-canonical variants redirect. |
| Content Provenance | Public Content is attributed to IdeaBoard or the IdeaBoard Editorial Team with real Published/Last-reviewed dates; no invented authors, ratings, or credentials. |
| Structured Data Contract | JSON-LD states only visible, verifiable facts: `Organization`/`WebSite` on home, `BreadcrumbList` on nav layers, `Article`/`TechArticle` only for real guides, `SoftwareApplication` only with sufficient product facts. |
| Public Pricing | Publicly verifiable pricing facts with a clear owner from the registry; the first cluster publishes no pricing and declares no `offers` in structured data. |
| First-party Organic Measurement | Discovery, crawl, and conversion measured via Search Console, Bing Webmaster Tools, Cloudflare/source logs, and server-side Workspace Activation events; no third-party tracking cookies or content surveillance. |
| Authenticated App Surface | Protected routes for login, account, Boards, Canvas, Admin, and tokenized shares; excluded from the sitemap and served `noindex`. |
| Public URL Integrity | Only published, existing Public Pages return `200`; missing, withdrawn, or moved URLs return real `404`, `410`, or explicit `301`; never a shared SPA shell faking success. |
| Public Content Lifecycle | Only `draft`, `published`, `retired`; builds publish `published` only; `retired` projects to `301` or `410` by replacement. |
| Initial Product URL Set | First release publishes only `/` and `/use-cases/pattern-ideation`; Pattern Generation, Style Variation, Style Fusion, Partial Editing, and Image Upscaling stay `draft` until credible supporting evidence exists. |
| Public Page Content Contract | Every published page needs an opening answer, target Visual Designer, real workflow steps, inputs/outputs/limits, related internal links, a Workspace Activation CTA, and Content Provenance, consistent with metadata/JSON-LD. |
| Content Cluster Link Graph | Pattern Ideation is the first-cluster hub; the first home links only to it; Capability Pages link back to the hub and to adjacent workflow neighbors once published. |
| Anonymous Public Content | Body, steps, inputs/outputs/limits, and metadata need no login; only the Workspace Activation CTA enters auth. |
| Intent-Preserving Handoff | Public Page CTAs carry the Use Case/Capability intent into the Workspace start flow without changing the canonical URL or sitemap. |

## Public Seams
- `https://ideaboard.art/` canonical root; all Public Pages use stable lowercase `kebab-case` paths with no trailing slash.
- `Public Fact Registry` single source for capability facts; Editorial Content cites it and never redefines it.
- `Repository-reviewed Publishing` content, metadata, structured data, and sitemap ship via repo review and build.
- `Intent-Preserving Handoff` CTA intent affects only the in-app start flow, never the canonical URL or sitemap.
- `First-party Organic Measurement` discovery and activation measured without third-party cookies.

## Invariants
- Public Site and Workspace routes stay separate; Workspace routes carry no acquisition duty.
- Pattern Ideation is the flagship Use Case; it never promises production-ready seamless repeat or print-production workflow.
- Pages are English-only at launch; never ship mixed-language pages, partial translations, or machine-translated alternates.
- Never treat search citation access as training access, or vice versa.
- Never use one rendering mode for all routes; never serve bot-specific rendering.
- Registry facts are never sourced from Admin prompts, discovered at runtime, or duplicated across metadata.
- Never ship generated SEO copy, hidden crawler text, or prompt templates as Editorial Content.
- Publishing is never driven by a runtime SEO CMS, prompts, or unreviewed copy.
- Content never lives in query or hash params; duplicate locale roots and trailing-slash variants are not allowed.
- Never use synthetic authority, anonymous AI copy, or fabricated reviews.
- JSON-LD facts must be page-visible and verifiable; never hide claims, fabricate ratings, or bait commercial FAQ rich results.
- First cluster publishes no pricing and no `offers`; never show admin-only, stale, or implied-free pricing.
- Never use cross-site profiles, session replay, or prompt/content surveillance.
- Authenticated surfaces stay out of the sitemap with `noindex`; never make workspaces searchable; robots alone is not privacy.
- Never serve soft 404s, `200` on unknown routes, or blanket SPA fallback.
- Only `published` pages build; `retired` maps to `301` or `410`; no runtime publish state, scheduled CMS workflow, or stale sitemap entries.
- First release is `/` plus `/use-cases/pattern-ideation` only; no premature feature sprawl, thin capability pages, or unverified routes.
- Every published page satisfies the Public Page Content Contract; no metadata-only pages, unsupported claims, or fabricated evidence.
- First home links only to the Pattern Ideation hub; no orphan pages, indiscriminate cross-linking, vague anchors, or links to unpublished routes.
- Page bodies are never auth-gated and never client-only SEO bodies; no user-asset previews in public content.
- Handoff never uses query-shaped canonicals and never loses Use Case context.

## Non-goals
- No User Home, landing SPA, or marketing shell.
- No generic AI-user targeting; no complete production-suite promise.
- No signup/CTA-click as the conversion; only Workspace Activation counts.
- No thin feature-page collections or generated keyword pages.
- No training-data licensing via citation access.
