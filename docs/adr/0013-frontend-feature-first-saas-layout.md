# ADR-0013: Frontend feature-first SaaS layout

Owner: Frontend Architecture
Decision date: 2026-10-10
Affected contexts: Frontend feature contexts

## Context

Frontend 以 feature cohesion、pure core 和 control colocation 保持 ownership 清晰。Bounded-context ownership 由 [ADR-0005](./0005-frontend-ddd-reorganization.md) 定义，目录深度不决定 domain boundary。

## Decision

Feature/tool scope 按职责使用三个 zones：

1. `core/`：pure computation 与 types，无 Vue reactivity、Router、Pinia、TanStack Query 或 generated DTO dependencies。
2. `session/`：stateful orchestration 与 adapters，拥有 lifecycle、reactivity、IO、coordinate/stage adapters、apply pipelines 和 runtime handoffs，可依赖 Vue 与自身 core。
3. `ui/`：按 control colocate SFC、local helpers 与 specs；例如 `curves/`、`levels/`、`histogram/`。Cross-control contracts 放在 UI root 或 core。

每个 tool scope 通过 `index.ts` 暴露 named model/session/apply/UI seams。Scope internals 保持 private；cross-scope imports 只使用登记的 public entry 或 port，不 deep import。简单 feature 在额外分区不能改善 ownership 时可以保持 flat，并在 `CONTEXT.md` 说明。

Specs 使用 `.(unit|contract|component|browser).spec.ts`，与对应 module 或 control colocate。Pure computation 使用 unit specs；rendered behavior 使用 component 或 browser specs。

## Enforcement

- Dependency-cruiser 校验 module direction、pure core 与 cross-tool boundaries。
- `checkNamingGovernance.ts` 校验 naming 和 control-colocated directories。
- `checkTestGuardrails.ts` 校验 spec suffix、placement 与行为断言约束。
- `features/canvas/core/canvasDomainPurity.unit.spec.ts` 校验 Canvas 各 pure core scopes。

Frontend 结构总契约见 `DESIGN.md`、`apps/web/DESIGN.md` 和对应 feature `CONTEXT.md`。改变 zone ownership、pure dependencies 或 public seams 时需要同步更新 contract specs 与文档。

## Consequences

Feature 内部的计算、编排和 UI ownership 保持明确；跨 scope 消费者依赖稳定的 public seams。新增分区必须改善 ownership，不能仅为保持目录形式一致而增加层级。

## Status

Accepted
