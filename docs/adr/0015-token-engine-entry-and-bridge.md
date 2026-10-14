---
owner: Web
decision_date: 2026-10-13
supersedes: none
---

# Token Engine Single Entry and Runtime/Utility Bridge

## Context

Runtime values 与 Tailwind utility keys 的职责不同。Standard semantic utilities
必须跟随 dark mode 和 subtree 中的有效 runtime value。Token imports、base rules
和 utility mappings 需要明确 owner，才能在编辑时保持这些 resolution rules。

## Decision

1. `src/theme/theme.css` 是唯一 Tailwind entry，拥有唯一的
   `@import 'tailwindcss'`、按依赖顺序排列的 token imports
   （primitives → semantic → canvas → dark）、
   `@custom-variant dark (&:where(.dark, .dark *))` 和 namespace bridge。
   `src/app/style.css` 是 `components.json` 指定的 entry，负责导入 theme、base 和
   atoms，并拥有 app-wide utilities、reduced-motion overrides、toast styling 和
   shell rules。
2. Standard runtime roles 使用 bare Tier 2 names（`--background`、`--primary` 等），
   light/dark assignments 由 `tokens/semantic.css` 的 `@layer base` 唯一拥有。
   对应的 `--color-*` utility keys 使用 `@theme inline` 映射到 runtime roles，
   使 `bg-primary` 输出 `var(--primary)`，在两种 mode 和 subtree 中跟随有效的
   base-layer value。Domain utility values 使用 plain `@theme`；Canvas declarations
   和 domain dark mode overrides 分别由 `tokens/canvas.css`、`tokens/dark.css` 拥有。
3. Consumers 直接使用 canonical runtime names，不保留 compatibility aliases 或
   shims。TS fallback bridges（`adminThemeTokens.ts`、`taskMetricTokens.ts`）与 CSS
   使用相同名称。
4. Authored colors 使用 sRGB representation。任何声称保持等价的 representation 或
   ownership 变更都必须满足 visual comparison contract；不能把 rendering differences
   视为 conversion noise。
5. Contrast readers 沿 token references 解析当前 hex leaves，拒绝不支持的 solid-color
   representations。Translucent surface values 必须先与 backing surface 合成再测量；
   保持 pair coverage、thresholds 和 ordering assertions。

## Consequences

- Standard roles 只有一个 runtime owner；`tokenEngine.browser.spec.ts` 验证
  dark/subtree resolution。
- 禁止 `dark:` color patches；semantic roles 已覆盖两种 mode。
- 既有 Canvas `@theme` entries 和 base-layer values 由
  `token-alias`/`dark-base-closure` 固定，不得新增重复声明。
- Visual acceptance 由人工执行。`test:visual-compare` 先校验完整 manifests，
  再比较提供的 capture evidence，不作为 CI acceptance gate。
  RGB channel tolerance 为 1/255；geometry、typography、stacking、hit bounds
  和 alpha 必须完全一致。

## Status

Accepted
