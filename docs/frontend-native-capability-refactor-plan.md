# Frontend Native Capability Refactor Plan

## 背景

Canvas 主路径已经完成一轮 Vue Flow / VueUse / browser native 能力收敛：

- Vue Flow 已成为 graph interaction 的主事件源。
- `nodes/edges` 与 `fullNodes/fullEdges` 已收敛为同一组 Vue Flow bound refs。
- 普通 selection、Delete / Backspace、MiniMap、handle placement、strength range、global listener cleanup、pointer drag session 等已完成主线迁移。
- 本轮已继续执行 mask raster compositing、viewport runtime、image editor chrome anchoring、node overlay utility、histogram threshold range 和小型 VueUse / Vue 原生化收尾。

本文件记录当前执行结果、保留的业务边界，以及后续仍可独立评估的低优先级项。

## 目标

1. 继续删除可被 Vue Flow / VueUse / Vue / browser / Canvas 2D 原生能力覆盖的自研平台层代码。
2. 保留 board snapshot、history、autosave、image workflow、mask editing、connection validation 等业务 contract。
3. 优先选择能真正降低复杂度的替换，避免只是把稳定业务逻辑换成更隐晦的 native wrapper。
4. 每个替换都必须接回现有 transaction / dirty / history / autosave 边界。

## 非目标

- 不把 domain snapshot 整体替换成 Vue Flow `toObject()` / `fromObject()`。
- 不用 VueUse `useRefHistory` 替换 `useFlowHistory` / `useFlowTransactions`。
- 不把 connection validation、port limit、stale propagation 交给 Vue Flow 基础 connection API。
- 不用 VueUse `useDraggable` 整体替换 `usePointerDragSession`。
- 不为了“原生化”改变 canvas 的现有手感、mask 语义或 image editor workflow。

## 本轮执行结果

### Completed

1. `maskPaintingCanvas.ts`
   - `invertMaskPreviewCanvas` 改为 Canvas 2D `destination-out` compositing。
   - `buildMaskCommitResult` 改为复制 alpha 后用 `source-in` tint execution / preview mask，删除 RGB 重写循环。
   - `featherMaskPreviewCanvas` 保留现有 box blur，最终 tint 改为 compositing。
   - `drawMaskRasterPreviewToCanvas` 继续保留 `alpha > 0 -> 255` 的 reload 二值化语义。
   - 新增 canvas fixture 测试覆盖 invert、feather、commit result、empty threshold、reload threshold。

2. `useCanvasViewportRuntime.ts` / `CanvasPage.vue`
   - `zoomLevel` 改为从 Vue Flow `viewport` ref 派生。
   - 删除 `useFlowViewportControls.ts`、`lastFlowX/Y/Zoom`、epsilon diff fallback 和 `applyFlowTransform` 桥接。
   - viewport persistence 改走 Vue Flow `@viewport-change-end`。
   - keyboard pan 改用 Vue Flow `panBy(delta)`，`panBy === false` 时不提交 dirty。
   - frame probe 的 `move-start` / `move` / `move-end` lifecycle 保留。

3. `useImageEditorViewportController.ts`
   - `window.resize` 改为 VueUse `useEventListener`。
   - anchor update 改用现有 `useRafScheduler`。
   - canvas wrap 和 chrome target resize 改用 `useResizeObserver`。
   - chrome anchor measurement 集中为 `measureImageEditorChromeAnchor`。
   - `resolveImageEditorFocusTarget` 的业务几何保留。

4. `useNodeToolPopoverGroup.ts`
   - 手写 `window pointerdown/keydown` 改为复用 shared `useDismissableLayer`。
   - 用 group token 标记 grouped roots，保留 active popover、outside dismiss、Escape 行为。

5. `ImageEditorHistogramPlot.vue` / `ImageEditorThresholdControl.vue`
   - threshold 单 marker 改用透明原生 `<input type="range">` 承接 keyboard、pointer、ARIA。
   - 视觉 marker 保留现有样式和 focus ring。
   - levels 多 marker 保留 button slider 逻辑，避免重叠 marker 和 min/max 约束复杂化。

6. Utility 收尾
   - `useMaskEditorChromeState` 改用 VueUse `useFileDialog`。
   - `ImageEditorCurveGraph` 改用 function template refs / ref Map 做 point focus lookup。
   - `ImageMaskEditorOverlay` brush cursor 优先用 `PointerEvent.offsetX/Y`，保留 rect fallback。
   - `useFlowNodeDragInteractions` 用 `queueMicrotask` 表达 drag stop commit 调度，并保留 latest-token 取消语义。
   - `useSortableMediaInputs` 改用 Sortable `oldDraggableIndex/newDraggableIndex`，保留 `oldIndex/newIndex` fallback，不再从 DOM `dataset.imageId` 反推顺序。

### Deliberately Not Changed

1. `ctx.filter='blur(...)'` 未替换当前 box blur：native blur 与现有 separable box blur 视觉不同，且当前 test environment 无法可靠验证真实 Canvas filter。
2. `useAutoResizeTextarea` 未替换为 `useTextareaAutosize`：当前 metrics `naturalHeight` / `singleLineHeight` 直接服务 `count-stacked`，VueUse 替换不能明显降低复杂度。
3. curve / histogram grid 未替换为 SVG `<pattern>` 或 CSS gradient：收益低，当前不是主要复杂度来源。

## 剩余可评估项

1. Canvas blur native 化：只有在能做真实浏览器视觉回归时，再评估 `ctx.filter='blur(...)'` 替换 box blur。
2. Grid 绘制样式化：curve / histogram 的 `createGridPath` 可改 SVG `<pattern>` 或 CSS gradient，但 ROI 低。
3. Node popover 组件化：可继续评估 PrimeVue `Popover` 或 browser native `popover`，前提是实测 canvas transform、Teleport、z-index、light dismiss 全部稳定。

## Explicitly Deferred

以下点已审过，但暂不进入主线：

1. `useFlowHistory` / `useFlowTransactions` -> VueUse `useRefHistory`：当前绑定 autosave、drag transaction、baseline、domain snapshot patch，不是普通 ref history。
2. `flowToCanvasBoardSnapshot` / `canvasBoardSnapshotToFlow` -> Vue Flow `toObject()` / `fromObject()`：业务 snapshot schema、后端持久化、pending image result 都是 domain contract。
3. connection mutation 全量 native 化：Vue Flow `addEdge` / `updateEdge` 只能替换小片段，port 语义和输入上限仍是项目逻辑。
4. `usePointerDragSession` -> VueUse `useDraggable`：现有 pointer capture、`lostpointercapture`、pointerId 过滤、cancel / end payload 是共享交互 contract。
5. mask brush engine -> `ctx.stroke()` / CSS / SVG：soft hardness、square brush、erase compositing 需要 raster stamp。
6. mask undo / history -> native canvas undo：浏览器没有可靠原生 undo，当前 `getImageData` 快照语义清楚。
7. image editor curve graph 核心交互 -> `<input type="range">`：2D point add / move / delete / hit radius 不适合 slider。
8. image extend overlay 8-anchor resize -> CSS `resize` / HTML Drag and Drop：边界约束、多方向 handle、move / resize session 需要自定义 pointer。
9. native `popover` / PrimeVue `Popover` 全量替换 node tool popover：可能受 canvas transform、Teleport、z-index、light dismiss 影响，先只复用 dismissable lifecycle。
10. CSS cursor 全量替换 mask brush cursor：大 brush、高 DPI、动态 shape / hotspot 存在浏览器限制。

## Verification

本轮执行后已运行：

```bash
cd apps/web && npm run test:run
cd apps/web && npm run build
cd apps/web && npm run lint
```

建议手动回归：

- mask preview、invert、feather、commit result、reload 后视觉一致。
- pan、zoom、fit view、MiniMap 操作后 viewport 恢复准确。
- image editor / mask editor 打开时 chrome anchoring 不漂移。
- aspect ratio / output count popover outside click、Escape、focus 不回归。
- prompt composer autosize 后 output count stacking 不回归。
- media input reorder 在 mouse / touch fallback 下顺序正确。
- histogram threshold slider 的 pointer、keyboard、focus、ARIA 行为正常。
- board 切换、刷新后 viewport、graph、transient UI 恢复稳定。
