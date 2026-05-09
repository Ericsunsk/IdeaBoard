# Canvas -> Vue Flow 迁移审计报告（收尾版）

更新日期：2026-05-08

## 当前状态（代码已落地）

- `WorkspaceScreen.vue` 已接入 `VITE_WORKSPACE_FLOW_ENABLE_CULLING`，默认 `false`。
- 网格背景已从外层 `.canvas-grid` / CSS variables 方案迁移为 `.vue-flow__viewport` 背景方案。
- 已拆分 composables：
  - `useFlowNodeContextMenu`
  - `useFlowSelectionPreviewBar`
  - `useFlowCanvasDrop`
  - `useFlowBoardHeaderActions`
- 旧 canvas 相关文件已进入 staged 删除集合（按当前工作树状态收口）。

## 真实残余项

### P1：用户浏览器验收（必须）

1. 网格视觉一致性
- 在快速缩放、平移、拖拽节点时，确认网格与节点运动感知一致，无漂移/抖动。

2. 大画布交互稳定性
- 在 100+ 节点画布下，确认连线、拖拽、框选、删除、缩放行为稳定。

3. Culling 开关专项
- 默认关闭（`false`）路径下确认行为稳定。
- 如需评估性能，手动开启 `VITE_WORKSPACE_FLOW_ENABLE_CULLING=true` 做对比回归。

### P1：运行时风险

- `onlyRenderVisibleElements` 启用后仍存在潜在端口/连线边界错位风险，需要专项回归后再决定是否默认开启。

### P2：后续可执行项（非阻断）

- 继续把 `WorkspaceScreen.vue` 做小步拆分（仅结构拆分，不改行为）。
- 按批次清理旧 canvas 残余代码（每批先用 `rg` 证明无引用，再删除）。

## 本轮不再列为待修的问题

以下事项已在当前代码中完成，不应再作为“未修复”重复派工：

- Phase A：culling feature flag 接入并默认关闭。
- Phase C：首批 composables 拆分完成（四个已落地）。
- Phase D：旧 canvas 关键文件已进入 staged 删除流程。

## 验证基线

- `git diff --check`
- `git diff --cached --check`（存在 staged diff 时）
- `cd apps/web && npm run build`
