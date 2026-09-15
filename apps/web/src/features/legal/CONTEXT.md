# Legal & Compliance

Legal 是平台的法务条款、隐私政策与合规页面上下文。它为匿名访问者与已注册用户提供可信赖的法律文本展示，支持版本追踪与打印布局。

## Language

**Legal Document**:
受版本号与生效日期约束的法律文本结构（`legalDocuments.ts`）。包含标题、版本（`version`）、生效日期与格式化段落。
_Avoid_: 散落于各 Vue 组件内手写的静态死文本，缺少版本跟踪

**Public Legal Shell**:
统一的法律文本展示壳（`PublicLegalPage.vue`）。提供文档目录跳转、可读性排版优化、打印样式适配与安全返回链接。
_Avoid_: 各协议页面重复编写外层导航与排版 CSS

## Module Ownership

| Module                           | Responsibility                                                | Public Interface            | Implementation Detail Boundary                             |
| :------------------------------- | :------------------------------------------------------------ | :-------------------------- | :--------------------------------------------------------- |
| `legalDocuments.ts`              | 事实源：各法律文本的数据结构、版本标记与多语言条款内容。      | `legalDocuments` 常量导出。 | 纯数据定义，不依赖 Vue。                                   |
| `components/PublicLegalPage.vue` | 法律文档排版壳：目录生成、响应式阅读器、顶部返回导航。        | 基础展示组件。              | 样式遵循阅读器排版，不引入业务侧复杂状态。                 |
| `pages/*Page.vue`                | 各独立法律路由页面（Terms、Privacy、Refund、AcceptableUse）。 | 路由页面出口。              | 薄装配层：读取 `legalDocuments` 并传入 `PublicLegalPage`。 |

## Rules

- 所有法律文本改动必须显式更新对应的版本号（`version`）与更新时间，以便于用户注册时与支付结账时的条款版本合规审计。
- 页面必须保持轻量、无鉴权依赖，完全支持服务端渲染（SSR）或静态落地展示。
