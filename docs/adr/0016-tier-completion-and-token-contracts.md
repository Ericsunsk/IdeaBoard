---
status: accepted
owner: Web
decision_date: 2026-10-13
supersedes: none
---

# Token 分层与运行时契约

## 背景

共享 UI、Admin 与 Canvas 需要不同的语义 roles，同时保持单一运行时 owner、明确的 light/dark assignments 和可验证的依赖方向。Tailwind utility keys 与运行时 CSS custom properties 的职责必须分开。

## 决策

1. `tokens/primitives.css` 是 mode-blind Tier 1，保存物理颜色、alpha recipes、radius、shadow、motion、type 与 stacking ingredients。Tier 2 负责语义选择；Tier 3 只引用 Tier 2，不保留 primitive consumer 豁免或 raw color pins。
2. `tokens/semantic.css` 与 `theme.css` bridge 定义共享及 domain Tier 2 roles；`tokens/admin.css` 定义 Admin 专属 roles；`tokens/canvas.css` 定义 Canvas scoped roles。Canvas workspace、tool、text、tooltip 与 progress 使用独立的共享语义角色，避免依赖 action palette。
3. `theme.css` 是唯一 Tailwind/token import 入口。标准语义 utility keys 使用 `@theme inline`，base-layer declarations 拥有 runtime roles；domain keys 使用 `@theme` 定义 light defaults（包括 Admin），base-layer declarations 提供 light/dark overrides。Graph 先解析 `@theme` defaults，再应用 base-layer overrides 和 CSS declaration 顺序，不把 inline mappings 当作运行时声明。
4. `tokens/motion.css`、`tokens/type.css`、`tokens/z.css` 分别拥有 motion、typography 与 stacking 语义。Radius 在所有 surface 共用 `control`、`panel`、`overlay`、`pill` 四个角色。完整 shadow recipes 保留几何关系。
5. `lint:token-graph` 强制 defined references、acyclic graph、Tier direction、显式 mode parity 与 Contrast floors。Action normal/hover/pressed 文字最低 4.5:1；chart marks 与 sidebar focus ring 最低 3:1；body/card/sidebar 文字最低 7:1。无法解析或非有限颜色按失败处理。Component primitive references 由 style governance 禁止，包含 whitespace 与 fallback 写法。
6. Fine/coarse pointer 使用普通控件的独立 hit-target profiles，precision handles 使用独立 profile。Existing overlays 在 `(pointer: coarse)` 下选择 coarse profile；inline overlays 只扩展 block axis。Compact Admin real-box controls 通过最小尺寸参与布局。
7. Mode-varying shadows 在消费者处经 `var(--shadow-*)` 解析。Native `shadow-*` 只用于 mode-blind recipes 或保持运行时变量引用的 recipes，防止 build-time snapshot 固定 light paint。

## 结果与约束

Token ownership、CSS runtime resolution 和治理门形成统一契约。新增颜色须先定义物理 ingredient 与有意义的 Tier 2 role，再由 scoped roles 或组件使用；不得增加 retained-value pins、compatibility aliases 或 component primitive 豁免。代码检查负责静态契约，浏览器视觉验收由人工完成。
