# Frontend Native Capability Refactor Plan

## 背景

当前 canvas 前端已经大量使用 Vue Flow，但仍有一部分交互状态由项目自己维护：

- `nodes/edges` 与 `fullNodes/fullEdges` 之间存在 selection、position、remove 等状态同步。
- Delete、selection、viewport、MiniMap、global keyboard listener、ResizeObserver 等逻辑有较多手写代码。
- Vue Flow、Vue 3.5、VueUse 已经提供了一部分可直接承接这些行为的原生能力。

本计划的目标不是简单替换 helper，而是减少自研交互平台层，把项目代码收敛到业务语义：board persistence、history、autosave、image workflow、transient UI。

## 目标

1. 让 Vue Flow 成为 canvas graph interaction 的主事件源。
2. 用 Vue Flow 的 `nodesChange` / `edgesChange` 承接 selection、position、remove 等 graph change。
3. 将项目的 history、dirty、autosave 接在统一 transaction 层，而不是散落在多个事件 handler 中。
4. 用 VueUse 标准化 global listener、ResizeObserver、RAF scheduler 等生命周期代码。
5. 删除能被 Vue Flow / Vue / VueUse 原生能力覆盖的自研代码。

## 非目标

- 不重写 image workflow、task runtime、board persistence 的领域语义。
- 不把 domain snapshot 整体替换成 Vue Flow `toObject()` / `fromObject()`。
- 不为了“使用原生能力”而替换已有清晰、稳定、业务定制强的几何逻辑。
- 不引入旧兼容分支。当前项目没有历史包袱，可以按最清晰的结构迁移。

## 当前关键位置

- Canvas shell: `apps/web/src/features/canvas/pages/CanvasPage.vue`
- Graph runtime: `apps/web/src/features/canvas/composables/useFlowCanvasRuntime.ts`
- Selection controller: `apps/web/src/features/canvas/composables/useFlowSelectionController.ts`
- Keyboard shortcuts: `apps/web/src/features/canvas/composables/useFlowKeyboardShortcuts.ts`
- Viewport runtime: `apps/web/src/features/canvas/composables/useCanvasViewportRuntime.ts`
- MiniMap: `apps/web/src/features/canvas/components/CanvasMiniMap.vue`
- Handle rail measurement: `apps/web/src/features/canvas/composables/useFlowCanvasNodeHandleRail.ts`
- Dismissable layer: `apps/web/src/shared/composables/useDismissableLayer.ts`

## Target Shape

### Graph Change Pipeline

Vue Flow emits graph changes:

```text
VueFlow nodesChange / edgesChange
  -> canvas graph change controller
  -> classify changes
  -> applyNodeChanges / applyEdgeChanges
  -> fullNodes / fullEdges
  -> transaction hooks
  -> history / dirty / autosave
```

Change 分类：

| Change type | 处理方式 |
| --- | --- |
| `select` | transient UI state，不进入 history，不 mark dirty |
| `position` | 拖拽过程中更新 graph，拖拽结束后进入 history / dirty |
| `remove` | 进入 transaction，处理 connected edges、history、dirty |
| `add` | 进入 transaction，history、dirty |
| `dimensions` | 默认不进入 domain snapshot，除非现有 persistence 明确需要 |

### Selection Ownership

Vue Flow owns normal selection:

- 普通左键选中
- Shift 多选
- 框选
- pane click 清空 selection

Project owns business side effects:

- context menu 打开前选中目标 node
- image editor / hand tool / locked 状态下禁止 selection
- pane click 关闭 transient UI
- selection export、delete、business command

### Keyboard Ownership

Vue Flow / VueUse owns lifecycle and primitive keyboard state:

- Delete / Backspace 可在 graph change pipeline 稳定后接回 Vue Flow `deleteKeyCode`
- Space / Alt pressed state 可逐步转为 VueUse 或 Vue Flow key handling
- listener cleanup 用 VueUse `useEventListener`

Project owns app commands:

- Cmd/Ctrl + S
- Cmd/Ctrl + Z / Shift + Cmd/Ctrl + Z
- V / H tool switch
- Escape 的业务取消逻辑
- inline selection delete

## Execution Plan

## Implementation Status

截至本轮收尾，当前 diff 的落地口径如下：

- Phase 1-4 已落地：Vue Flow change stream、selection ownership、Delete / Backspace 原生化、官方 MiniMap 瘦身。
- Vue Flow `applyDefault` 已恢复为默认行为；项目侧 `nodesChange` / `edgesChange` 只保留 full graph sync、remove cleanup、history / autosave side effect，不再复制 `applyNodeChanges` / `applyEdgeChanges`。
- `useFlowGraphChangePipeline` 与 `useCanvasViewportCommands` 已删除；当前目标是优先用原生行为，项目只补业务 transaction。
- Phase 5 已按 canvas 主路径收口：keyboard listener、dismissable layer、viewport resize observer、handle rail observer / RAF / timer、image editor mode、extend overlay、strength control cleanup。
- Phase 5 的通用 `usePointerDragSession` 已落地：统一 pointer capture、window move/up/cancel、lost capture、scope cleanup；mask painting、curve graph、histogram、extend overlay 仍各自保留业务坐标投影。
- High ROI audit 已在本轮执行：graph alias、handle rail native placement、strength range 原生化均已落地；后续只保留手动交互回归和更低 ROI 候选。
- Phase 6 是 Future 候选能力，不是本轮验收条件；只在写路径不会绕过 transaction、手感明确变好时再单独执行。

### Phase 1: 接入 Vue Flow change stream

目标：建立 graph change 主干，先不急着删所有旧逻辑。

任务：

1. 在 `CanvasPage.vue` 增加 `@nodes-change` 和 `@edges-change`。
2. 在 `useFlowCanvasRuntime.ts` 增加 graph change controller。
3. 使用 Vue Flow `applyNodeChanges` / `applyEdgeChanges` 更新 `fullNodes/fullEdges`。
4. 明确 change 分类，避免 `select` 进入 history / dirty。
5. 保留现有 drag transaction，但让它基于 change stream 结果提交。
6. 补测试覆盖 selection、position、remove、history、dirty。

验收：

- 节点拖动后位置保存正常。
- 选中、取消选中不触发 dirty。
- 拖拽结束只产生一次有效 history entry。
- 撤销重做仍能恢复节点位置。
- 切换 board 后 selection 不漂移。

### Phase 2: 收缩 selection controller

目标：让 Vue Flow 承担普通 selection，项目只保留业务 side effects。

任务：

1. 删除普通左键路径中的 `findNode` / `addSelectedNodes` / `removeSelectedElements` 手写操作。
2. 保留 context menu 选中目标 node 的特例。
3. 保留 pane click 关闭 transient UI 的逻辑。
4. 确认 `selectNodesOnDrag`、`selectionKeyCode`、`multiSelectionKeyCode` 与 active tool / lock 状态一致。
5. 删除 `fullNodes/fullEdges` 中不再需要的 selected flag 手写同步。

验收：

- 单选、Shift 多选、框选行为稳定。
- 节点拖动后不会丢失真实 selection。
- 右键菜单仍能选中对应 node。
- hand tool、image editor lock 状态不误选。

### Phase 3: Delete / Backspace 原生化

目标：把删除选中元素纳入 Vue Flow lifecycle，但保留业务 transaction。

任务：

1. 在 Phase 1 稳定后评估启用 `deleteKeyCode`。
2. 将 Vue Flow remove changes 映射到项目删除 transaction。
3. 删除 `useFlowKeyboardShortcuts.ts` 中的普通 Delete / Backspace graph delete 分支。
4. 保留 inline selection delete 和业务 Escape 逻辑。
5. 删除时统一关闭 transient UI、清 connected edges、mark dirty、push history。

验收：

- Delete node 会删除相关 edges。
- Delete edge 正常进入 history。
- Undo 能恢复删除内容。
- 文本输入、prompt 编辑、inline selection 中 Delete 不误删 graph。

### Phase 4: MiniMap 原生化瘦身

目标：尽量使用官方 `@vue-flow/minimap`，减少自定义 viewport geometry 和 pointer layer。

任务：

1. 试用官方 `pannable` / `zoomable` / `inversePan` / mask props。
2. 验证 MiniMap 操作是否触发 Vue Flow `move` / `moveEnd` / `viewportChangeEnd`。
3. 如果原生事件足够，删除自定义 interaction panel。
4. 如果原生 mask 能满足视觉，删除自定义 viewport fill panel。
5. 删除不再需要的 `canvasMiniMapGeometry`。

验收：

- 用户点击开关即可显示 MiniMap。
- MiniMap 拖拽视口能移动 canvas。
- canvas 移动平滑，不出现明显抖动。
- viewport persistence 仍然准确。
- 视觉保持极简灰色风格。

### Phase 5: VueUse 生命周期标准化

目标：减少手写 listener、observer、timer、pointer session cleanup。

任务：

1. 用 `useEventListener` 替换 global keyboard listener 注册/释放。
2. 用 `useEventListener` 标准化 `useDismissableLayer` 的 pointer / key listener。
3. image extend overlay、image editor mode 中的 global listener 逐步改为 `useEventListener`。
4. wrapper size observer 改为 `useResizeObserver`。
5. handle rail 先只替换 observer cleanup，不直接改变 offset 几何算法。
6. 抽一个项目内 `useRafScheduler`，统一 one-shot RAF batching。
7. 抽一个轻量 `usePointerDragSession`，统一 pointer capture、move/up/cancel、cleanup；mask painting、curve graph、histogram、extend overlay 的坐标投影仍留在各自业务模块。
8. 用 `useTimeoutFn` / `useDebounceFn` 收敛长按、延迟显示、短暂提示类 timer。

验收：

- 所有 listener 在 scope dispose 时自动释放。
- ResizeObserver cleanup 不再手写分散逻辑。
- handle rail 位置不回归。
- keyboard shortcut 行为不变化。
- pointer capture 在 pointerup、pointercancel、lostpointercapture、scope dispose、异常打断路径下都能释放或正确结束。
- strength control、mask feather、layer progress 等延迟 UI 不泄漏 timer。

### Phase 6: 补充低风险原生能力

目标：用原生能力优化 read model 和小型 UI utility，不碰核心业务写路径。

状态：Future。本轮不把这些候选项计入完成标准，避免为了“补原生能力”绕过已有 transaction / persistence 语义。

候选：

1. `useNodeConnections`：用于节点 UI 读取连接关系，不替代 persistence adapter。
2. `useNodesData` / `getNode` / `getEdge`：用于只读 selector。
3. `updateNodeData`：等 Phase 1 transaction 主干稳定后，再包一层接入 history / dirty。
4. `useLocalStorage` / `useStorage`：auth 这类简单对象优先；canvas board state 继续保留 schema 校验。
5. `useTextareaAutosize`：可替换 textarea autosize，但必须保留 `naturalHeight` / `singleLineHeight` metrics。
6. Vue Flow interaction props：评估 `autoPanOnNodeDrag`、`autoPanOnConnect`、`autoPanSpeed`、`paneClickDistance`、`nodeDragThreshold`、`elevateNodesOnSelect`、`elevateEdgesOnSelect`。
7. focus 原生化：谨慎评估 VueUse `useFocusWithin` / `useActiveElement` 或 CSS `:focus-within`，但当前 handle rail 使用 `:focus-visible` 语义，不能机械替换。
8. async cleanup：未来重构 board load/save 时评估 Vue 3.5 `onWatcherCleanup` 和 browser `AbortController`；当前 `loadSequence` / `saveSequence` 不建议直接替换。

验收：

- read model 代码减少重复 `find` / `Map`。
- 写操作没有绕过 transaction。
- storage parse error 和 fallback 行为不回归。
- 交互体验 props 只在手感明确变好时保留，避免为了启用原生能力改变用户习惯。
- focus 显示语义不从 keyboard-visible 退化成所有 focus 都显示。
- async stale commit 防护不回归。

## Next Native Simplification Audit

本节记录在 Phase 1-5 收口后，继续审计出的原生化候选。排序按 ROI，而不是“看起来更原生”的程度。

### Completed High ROI

1. `useFlowCanvasRuntime.ts` 的双 graph 同步
   - 已执行：`nodes/edges` 与 `fullNodes/fullEdges` 收敛为同一组 Vue Flow bound refs；删除 Map based item sync、position / selection microtask sync、pending remove sets。
   - 保留桥接：Vue Flow native delete 后仍需要一个小的 remove microtask commit，用于在原生删除应用后写入 history、dirty、autosave side effect。
   - 后续风险：手动 smoke Delete node / edge、undo / redo、drag stop autosave、board switch selection，确认 native 时序没有漏业务 transaction。

2. `useFlowCanvasNodeHandleRail.ts` / `FlowNodeHandles.vue`
   - 已执行：handle rail 下沉到实际 node stage wrapper；`<Handle>` 继续由 Vue Flow 定位，CSS 负责命中区；删除 rail bounds、frame lookup、ResizeObserver、RAF measurement、`handleRailStyle`。
   - 保留桥接：`FlowNodeHandles` 仍在 port / snapshot 变化后请求 `updateNodeInternals`，避免连接锚点 stale。
   - 后续风险：手动 smoke hover、focus-visible、touch hint、image editor / expand overlay 下的 handle hit area。

3. `useFlowImageActionStrengthControl.ts` / `useMaskFeatherStrengthControl.ts`
   - 已执行：强度控制改为原生 `input[type=range]` 处理拖拽、键盘和 ARIA；长按入口改为 VueUse `onLongPress`；删除 global `mousemove` / `mouseup`、rect ratio、长按 timer 自研逻辑。
   - 保留桥接：click suppression、pointerup / change commit、业务 action dispatch 仍在项目 composable 中，避免 range 原生事件绕过 image workflow。
   - 后续风险：手动 smoke 普通点击、长按进入 range、拖动释放只提交一次、键盘调整提交、hover / disabled 状态。

### Medium ROI

1. `useFlowSelectionController.ts` 继续瘦身
   - 现状：普通 selection 已主要交给 Vue Flow，controller 仍包装 pane click、node click、context menu selection。
   - 可能方向：pane / node click 只做 transient UI dismiss，保留 context menu 选中 helper。
   - 收益：低风险继续减小 controller interface。
   - 风险：右键菜单选中、hand tool、locked / editor 状态需要回归。

2. `useRafScheduler.ts`
   - 现状：集中提供 one-shot latest-wins RAF，主要服务 handle rail 和 node internals refresh queue。
   - 可能方向：换成 VueUse `useRafFn({ immediate: false })` 或局部 one-shot helper。
   - 收益：删除一个小型通用 scheduler 和 spec。
   - 风险：要保留 latest-wins、scope dispose cancel、无 RAF fallback。收益不如 High ROI 项。

### Low ROI / Defer

1. `usePointerDragSession.ts`
   - VueUse `useDraggable` / `usePointerSwipe` 不贴合当前 pointer capture、lostpointercapture、payload、business coordinate projection。
   - 若要继续降复杂度，优先在 `ImageEditorCurveGraph.vue` / `ImageEditorHistogramPlot.vue` 内局部内联原生 pointer capture；mask painting / extend overlay 暂不拆。

2. `useFlowNodeDragInteractions.ts`
   - 可合并进 native `nodeDragStart` / `nodeDragStop` handler，但 Alt duplicate、history、autosave、probe 都是业务语义。
   - 预计多半只是换壳，不优先。

3. connection / node mutation
   - Vue Flow `addEdge` / `updateEdge` / `getConnectedEdges` 只能替换小片段。
   - port 语义、输入上限、stale propagation、domain snapshot 仍是项目逻辑，不建议为了原生化硬换。

## Explicitly Deferred

以下点已审过，但暂不进入主线：

1. `snapToGrid`：除非产品明确要网格对齐，否则会改变自由画布手感。
2. `toObject()` / `fromObject()` 整体替换 snapshot adapter：domain snapshot 需要连接 id、port limit、renderability 过滤，不等价。
3. VueUse `useDraggable` 整体替换自定义拖拽：image editor、mask、curve、histogram、extend overlay 都有业务坐标投影；当前只采用项目内 `usePointerDragSession` 抽生命周期，不接管坐标语义。
4. `onClickOutside` 整体替换 `useDismissableLayer`：当前有 `ownerDocument`、ignore selector、capture phase，需要保留更深的 interface。
5. `useAsyncState` 整体替换 board persistence：当前 stale sequence 是核心正确性逻辑。
6. Canvas / SVG 绘制整体替换：mask overlay、curve graph、histogram plot 是业务核心渲染，不应为了换库牺牲可控性。

## 建议拆分

推荐按以下顺序执行：

1. Phase 1: Vue Flow change stream 主干
2. Phase 2: selection controller 收缩
3. Phase 3: Delete / Backspace 原生化
4. Phase 4: MiniMap 原生化瘦身
5. Phase 5: VueUse 生命周期标准化
6. Phase 6: 低风险原生能力补充

前三个 Phase 是同一条主线，应连续执行并重点回归。后三个 Phase 可以拆给子代理并行处理，但需要控制文件 ownership，避免同时改 `CanvasPage.vue`。

## Verification Checklist

每个 Phase 至少运行：

```bash
cd apps/web && npm run test:run
cd apps/web && npm run build
```

重点手动回归：

- 新建 board 后拖入图片、删除、再次拖入。
- 单选、Shift 多选、框选、空白点击取消。
- 选中节点后拖动，选中态保持稳定。
- Delete node / edge，然后 undo / redo。
- hand tool 与 select tool 切换。
- image editor / mask editor / extend overlay 打开时 canvas selection 不串扰。
- image editor curve、histogram、mask painting、extend overlay 的 pointer drag 不丢 capture；pointercancel / lostpointercapture 后不再继续响应 move。
- MiniMap 开关、拖动视口、缩放后显示。
- 大画布边缘拖拽 / 连线时如果启用 auto-pan，手感不能比当前更抖。
- board 切换、刷新后 viewport 和 graph 恢复。

## 风险

1. `select` change 如果误进入 dirty，会导致无意义 autosave。
2. `position` change 如果每帧进 history，会污染 undo stack。
3. Delete 如果直接交给 Vue Flow 而绕过 business transaction，会漏 connected edges、dirty、autosave。
4. MiniMap 官方交互如果不能触发现有 viewport persistence，需要通过 `viewportChangeEnd` 补桥接。
5. VueUse 替换 listener 时要保留 capture phase、ownerDocument、editable target suppression。

## 推荐第一刀

先做 Phase 1。

原因：

- 它是后续 selection、delete、drag、history、autosave 收敛的主干。
- 不先建立 change stream，后续只会继续在旧双写结构上打补丁。
- 完成后可以删除最多冗余，并降低之前 selection 不稳定类问题再次出现的概率。
