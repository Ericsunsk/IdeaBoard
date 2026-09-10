# Canvas Workspace

Canvas Workspace 是用户编辑 Canvas Board、编排节点、运行图像任务并就地编辑图像结果的上下文。

## Design Docs

- [GRAPH_RUNTIME_DESIGN.md](./GRAPH_RUNTIME_DESIGN.md)：Canvas Graph Runtime brief，定义 normalized graph store、transaction、Vue Flow adapter 和 canonical mutation path。

## Language

**Canvas Workspace**:
一个 Board Session 的交互工作区，包含 graph interaction、overlay、image tools 和 execution feedback。

**Canvas Workspace Runtime**:
Canvas Workspace 的 page-level coordination runtime，由 `useCanvasWorkspaceRuntime()` 创建，负责组合 Board Session、Vue Flow Pane、persistence、viewport controls、graph editing、image tools、image workflow 和 node interaction。

**application/workspace public seam**:
`useCanvasWorkspaceRuntime()` 暴露给页面的稳定 seam，返回 `{ viewModel, commands }`。页面只通过这个 seam 读取 UI state 或触发动作；内部组合细节留在 `application/workspace/internal/createCanvasWorkspaceRuntimeParts.ts`。

**runtime ownership seam**:
Board/Flow assembly 按 `boardSession`、`flowPane`、`graphEditing` 和 `boardChrome` 暴露 typed capability。不得重新引入 `graphRuntime` 之类只聚合 callbacks 的 shallow coordinator，也不得把 internal lifecycle cleanup command 暴露给 Workspace。
_Avoid_: callback bag, compatibility export, escaped lifecycle cleanup

**Canvas Screen module**:
Canvas Workspace 的 screen-level interaction assembly，组合 Workspace Runtime、Vue Flow Pane、chrome、selection、group interaction、keyboard、dismissable UI、Node presentation intent router 与 Board Session lifecycle。CanvasPage 只向它传递 route、user identity 和 logout integration。
_Avoid_: CanvasPage 里散落的 interaction refs、native event callbacks、keyboard/lifecycle wiring

**Canvas surface UI**:
Canvas Screen module 拥有 Context Menu 的 visibility、相对 Canvas container 的 screen position 和创建时的 flow coordinate snapshot。Graph editing 只接收 normalized node creation intent，不保存 DOM-relative overlay state。
_Avoid_: Graph editing-owned overlay state, recomputing creation coordinates after viewport changes

**Canvas Chrome module**:
Canvas Workspace 的 chrome layout shell，负责把 Board header、right action/account group、left tool palette、bottom status/zoom controls 和 minimap anchor 组合成稳定的页面 chrome。CanvasPage 只传入 state/actions；Canvas Chrome module 决定这些 chrome 在哪里、如何响应 viewport、如何分层和如何隐藏。
_Avoid_: CanvasPage geometry, scattered top/right/bottom/left classes

**chrome geometry**:
Canvas Chrome module 拥有的布局几何 contract，包含 inset、anchor、z-index、pointer-events layering、fixed control dimensions、group gap、responsive placement 和 hidden/visible animation。
_Avoid_: route-local positioning classes, legacy chrome class variants

**chrome action group**:
Canvas Chrome 中的一组相关操作入口，例如右侧 action/account group 或左侧 tool palette。调用方传入按钮状态、disabled/loading state 和 action callbacks；Canvas Chrome module 决定 grouping、slot sizing、spacing 和 placement。
_Avoid_: hand-authored page controls

**status controls**:
Canvas Chrome 中表达 workspace/session/viewport 状态的控件，例如底部 zoom/status controls 和 minimap anchoring。Status controls 可以有自己的内部尺寸 contract；不要把 zoom 内部按钮强行并入 page chrome round-control 尺寸。
_Avoid_: graph runtime state source

**viewModel**:
Canvas Workspace Runtime 的 read side，暴露页面渲染和 binding 所需的 reactive state。业务决策不能从 adapter projection 反推 domain facts。

**commands**:
Canvas Workspace Runtime 的 write side，暴露用户动作、Vue Flow events、persistence、viewport、graph editing、image tools、image workflow 和 node interaction handlers。

**flowPane adapter data**:
`viewModel.flowPane.nodes` 和 `viewModel.flowPane.renderEdges` 是给 Vue Flow 的 adapter projection，只用于 `<VueFlow>` binding，不是 Canvas Board 的业务事实来源；`flowPane.edges` 保持 semantic graph edges，供 graph/persistence runtime 使用。

**Canvas Board**:
持久化的 graph document，由节点、连接和 board metadata 组成；viewport 是按 user+board 存在浏览器本地的 UI state，不属于 board content。
_Avoid_: Vue Flow graph, canvas state

**Board Session**:
单个 Canvas Board 的打开、保存、autosave、preview 和导航生命周期。

**Board Sync Coordinator**:
Board Session 的 load/save orchestration boundary，负责 autosave state、remote sync failure/conflict recovery、media stabilization/rebase 与 revision-gated reconciliation。它只返回 typed load/save result，不拥有 Graph projection、Vue Flow binding 或 history baseline；由 Workspace Graph Runtime 根据结果应用 hydrate/reconciliation。
_Avoid_: coordinator 直接调用 `commitGraph`、替换 renderer snapshot、维护 history baseline 或依赖 Vue Flow projection。

**Board Save Recovery**:
Board Session 保存时 local state/meta 已写入，但 remote sync 失败后的可恢复状态。actionable failure 必须通过 Toast 暴露：remote sync failure 是 warning，名称冲突或一般保存失败是 error；silent autosave success 保持无提示。dirty/retry/conflict state 继续驱动 network/re-auth automatic recovery，但不再要求 persistent autosave status block。
_Avoid_: Silent save success, invisible actionable failure, persistent autosave status block

**Vue Flow Pane**:
Vue Flow 提供的 graph interaction plane，负责节点布局、viewport movement、selection 和 native graph events。
_Avoid_: Canvas, board

**Canvas Flow Pane connection adapter**:
`commands.flowPane.connectionAdapter` 暴露的单一 connection seam。它把 Vue Flow `isValidConnection` / `onConnect` bindings 与 programmatic `connectIntent` 聚合；页面只绑定 adapter 的 `vueFlowBindings`，programmatic action 只传由 `CanvasGraphNodeId` 和 `CanvasConnectionPort` 组成的 normalized intent。已创建 connection 的 endpoints 不可变；需要调整位置时必须先删除再创建。
_Avoid_: page-level connection commands, Vue Flow event policy

**HTMLCanvasElement**:
浏览器 raster drawing surface，用于 mask preview、mask commit、histogram 和 rendered image adjustment 等像素操作。
_Avoid_: Canvas Board, Canvas Workspace

**Canvas Node**:
Canvas Board 中的 domain node，例如 prompt node、image node 或 fusion node。
_Avoid_: Flow node

**Flow Node**:
Canvas Workspace 挂载期间，Canvas Node 在 Vue Flow 中的 runtime representation。
_Avoid_: Canvas Node

**CanvasGraphStore**:
Canvas Graph Runtime 的 normalized graph state fact source，负责 O(1) node/edge lookup、hydrate/export 和 transaction commit summary。
_Avoid_: Vue Flow nodes array, persistence snapshot

**CanvasGraphTransaction**:
Canvas Graph Runtime 的 logical mutation boundary，用于把 drag、connect、delete、result materialization 等用户动作聚合为一次 commit。
_Avoid_: raw Vue Flow change batch

**CanvasVueFlowGraphAdapter**:
Canvas Graph Runtime 中把 CanvasGraphStore projection 到 Vue Flow interaction plane 的 adapter。
_Avoid_: graph domain store

**Canvas Graph Editing**:
Canvas Node creation、file/drop upload、media-input mutation、image preview 和 append-to-canvas 的 deep module。Screen 只发送 intent；upload、browser interaction 和 preview 通过 injected adapters 进入 implementation。
_Avoid_: private Flow commands, Screen-owned upload sequence, selection/export/interaction state

**Task**:
Canvas Workspace 提交到后端的 image generation 或 retouch job。
_Avoid_: Execution plan, pending node

**Execution Plan**:
把 Canvas Node 转成一个或多个 Task 的准备结果，包含 source images、action mode、sizing、prompt 和 mask inputs。
_Avoid_: Task

**Pending Result Node**:
代表已请求但尚未完成的 Task output 的临时 Canvas Node。
_Avoid_: Task, execution plan

**Image Workflow Execution**:
准备 image action request、提交 Task，并 materialize pending 或 completed image result 的能力。

**client_action_id**:
Canvas Workspace 为一次用户 generation action 生成的 idempotency key。同一次点击/网络重试复用它以避免重复 Task/provider cost；用户主动 rerun 必须生成新的 `client_action_id`。
_Avoid_: content fingerprint, Task ID

**Generation Degradation**:
provider unavailable 等外部能力不可用时的 Canvas recovery state。Generation command 可以短暂禁用并提示 retry，但 Canvas editing、selection、layout 和 local draft 保存必须继续可用。
_Avoid_: Canvas disabled state, permanent workflow shutdown

**Retouch**:
image node 的非破坏性 adjustment mode，包含 adjustment layers 和 rendered previews。
_Avoid_: Mask editing

**Inpaint**:
image node 的 mask authoring mode，包含 brush state、mask preview 和 committed mask data。

**Mask Draft**:
Inpaint 激活时的 in-progress mask state。
_Avoid_: Committed mask

**Committed Mask**:
用户应用 Mask Draft 后附着在 image node 上的 saved mask state。

**maskDataUrl**:
Canvas Workspace 和 node UI 使用的 preview-oriented mask image。
_Avoid_: executionMaskDataUrl

**executionMaskDataUrl**:
发送给 Image Workflow Execution 的 task-oriented mask image。
_Avoid_: maskDataUrl

**Image Tool Session**:
一个 Canvas Workspace 内对 Retouch、Inpaint、chrome state、node bindings 和 workflow actions 的 active coordination。
_Avoid_: Retouch domain, mask domain

**Apply Scope**:
一次 retouch apply 在 render、materialize、recipe 写入期间的存活断言。只有 `isCurrent()` 的 scope 才能触碰 graph；cancel/exit 会 settle 掉旧 scope，使迟到的 completion 失效。scope 追踪跨 generation，per-generation 的 cleanup 必须自己保证一次性。
_Avoid_: boolean applying flag, unguarded late completion

## Module Ownership

本节定义 Canvas module cleanup 的 ownership map。它描述当前边界，不要求为了对齐文档移动大量文件；后续重构应优先让代码逐步收敛到这些 public Interface。

| Module              | Responsibility                                                                                                                                                                                                                                              | Public Interface                                                                                                                                                                                                                                                                                                                                                                                  | Implementation Detail Boundary                                                                                                                                                                                                                                                                                                                                            |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `application/workspace` | Canvas Workspace 的 Facade runtime，组合 Board Session、flow pane、persistence、viewport controls、graph editing、image tools、image workflow 和 node interaction。                                                                                         | `application/workspace/index.ts` 导出的 `useCanvasWorkspaceRuntime(options)` 与 `CanvasWorkspaceRuntime` 类型。页面层只能读取 `runtime.viewModel`，并通过 `runtime.commands` 触发 runtime 能力。Board/Flow assembly 在内部按 `boardSession`、`flowPane`、`graphEditing`、`boardChrome` 分组，不提供通用 callback bag。                                                                                | `application/workspace/internal/createCanvasWorkspaceRuntimeParts.ts` 和各子 runtime 的组合顺序、local refs、dependency wiring 是内部实现。页面和其他 module 不应直接 import internal parts、访问 lifecycle cleanup command 或反推内部对象结构。                                                                                                                              |
| `presentation/screens`      | Canvas Workspace 的 screen-level interaction assembly，组合 pane、chrome、selection、group interaction、keyboard、dismissable UI、Node presentation intent router、Vue Flow rendering binding 与 Board Session lifecycle。                                  | `presentation/screens/index.ts` 导出的 `CanvasScreen`。CanvasPage 只传 route、router、user identity 与 logout integration；Screen 只通过 Workspace Runtime public seam 使用 runtime capability。                                                                                                                                                                                                          | Canvas Screen 内部的 UI refs、native Vue Flow event wiring、rendering policy、keyboard/leave guard、overlay close policy 与 intent router 都不属于 Page contract。Screen 直接拥有这些实现细节，不再通过第二个 Presentation interface 暴露或复制完整 callback graph。                                                                                                      |
| `domain/graph`     | Canvas graph mutation 的 canonical domain runtime，维护 normalized graph fact source、transaction、commit summary、hydrate/export 和 O(1) lookup/index。                                                                                                    | `domain/graph/index.ts` 导出的 `CanvasGraphStore`、transaction types、id helpers 和 summary types。调用方通过 transaction/summary 表达 mutation intent。                                                                                                                                                                                                                                         | `CanvasGraphStore` 的 `Map`/index 结构、transaction staging、projection patch 计算是实现细节。`domain/graph` 不 import Vue Flow，不保存 Flow Node/Edge shape，也不把 Vue Flow event semantics 当成 domain truth。                                                                                                                                                        |
| `application/editing`     | 由两个互补 deep module 协调 Canvas graph intent：`useCanvasGraphEditing()` 负责 Node creation、file/drop upload 与 media-input mutation；`useCanvasGraphInteractions()` 负责 selection、preview、append/export、Alt-drag duplication 与 interaction state。 | `application/editing/index.ts` 导出两个 runtime、browser/upload adapter types 和 browser adapter factories。页面只通过 `commands.graphEditing` 发出 intent；Node presentation handlers 由单一 `createNodeInteractions()` 创建。Selection identity 可来自 Flow adapter，但 preview/export/duplicate 的业务事实必须从 `CanvasGraphStore` 读取。                                                           | Upload preparation/freshness、DOM file picker、download dispatch、drop normalization、snapshot mutation、execution dirty propagation、preview dispatch 和 selection projection 属于 implementation。Node media/input upload 不进入 graph interaction module；Graph interaction 不从 Vue Flow `data.snapshotNode` 反推业务事实。                                           |
| `infrastructure/vue-flow-adapter`  | Vue Flow interaction plane 的 adapter boundary，承接 native node/edge/viewport events，并把它们转成 Graph Runtime transaction 或 adapter projection。当前是 logical submodule，主要通过 `application/workspace` 的 `flowPane` view/commands 暴露。              | `viewModel.flowPane.nodes` / `viewModel.flowPane.edges` 只用于 `<VueFlow>` binding；`commands.flowPane` 暴露 `onNodesChange`、`onEdgesChange`、单一 `connectionAdapter` 和 `nodeDragTransactions`。connection adapter 在 `infrastructure/infrastructure/vue-flow-adapter/index.ts` 作为 public seam 导出，聚合 `vueFlowBindings` 与 adapter-normalized `connectIntent`。                                                        | `@vue-flow/core` types/events、Flow Node/Flow Edge arrays、drag RAF projection、selection bridge、handle refresh 和 Vue Flow adapter composables 属于 adapter detail。页面业务逻辑和 graph domain 不应依赖这些细节。                                                                                                                                                      |
| `application/board-session`     | 单个 Canvas Board 的打开、保存、remote sync recovery、preview refresh、upload lifecycle 和 board id resolution；`Board Sync Coordinator` 编排 load/save state、media stabilization/rebase 与 revision-gated reconciliation。                                | `application/board-session/index.ts` 导出的 `useBoardSession()`、`createBoardSyncCoordinator()`、`BoardSession`、sync/load/save result types。Workspace runtime 通过该 seam load/save/refresh board，并由 Graph Runtime 应用返回的 snapshot。                                                                                                                                                     | `boardSessionRepository`、preview renderer、upload stabilization/recycle、remote sync error classification 是内部实现。Coordinator 不直接调用 Graph projection、Vue Flow adapter 或 history baseline；页面不直接调用 repository 或拼装保存流程。                                                                                                                          |
| `presentation/chrome`  | Canvas Workspace 的 chrome layout shell，拥有 Board header、right action/account group、left tool palette、bottom status/zoom controls、minimap anchoring、chrome geometry 和 interaction shell。                                                           | CanvasPage 传入 board/session/tool/account/status state 与 commands/actions；Canvas Chrome module 输出 chrome layout components。Provider-bound pieces such as minimap can be mounted inside the Vue Flow subtree through the module seam. 目标 public seam 应收口在 `presentation/chrome/index.ts` 或同等 feature-local barrel，暴露 chrome components/types，而不是暴露 positioning class recipe。 | top/right/bottom/left anchoring、inset、z-index、pointer-events、visibility animation、responsive placement、round-control slots、group gap、CSS class names 和 DOM grouping 是 module 内部 contract。调用方不手写这些 geometry，也不依赖 legacy chrome class variants。                                                                                                  |
| `execution`         | Image action 的执行 contract 和 planning boundary，把 Canvas Node/input 转成 `Execution Plan` 和 task request 所需的 typed params。                                                                                                                         | `application/execution/index.ts` 导出的 feature/action constants、execution contract builders、size/ratio helpers、`createImageExecutionPlanner()`、plan/result types。                                                                                                                                                                                                                                       | Feature-specific params assembly、media preparation、local inpaint helpers、task-runner 和 result-node materialization 不应泄漏成页面 API。`Execution Plan` 不是 `Task`，也不代表 pending UI state。                                                                                                                                                                      |
| `task-lifecycle`    | Canvas Task 的通用 cancellation policy owner，负责 task id normalization、bounded retry 和最终 failure reporting。                                                                                                                                          | `application/execution/task-lifecycle/index.ts` 导出的 `createCanvasTaskLifecycle()` 与 `cancelTasks()` command。Delete lifecycle 只提交由 Execution Runtime exact lease 解析出的 task ids；late-submit compensation 复用同一 command。                                                                                                                                                                                 | Task API adapter、retry delays、scheduler 和最终 warning 属于 module implementation。Workspace composition root 只装配 adapter，不实现 cancellation policy；删除与 stale-submit 链路不提供跳过 Task cancellation 的 fallback。                                                                                                                                            |
| `application/image-workflow`    | Canvas Workspace 中运行 image workflow 的 deep module：协调 Image Tool Session、direct action intent、execution planning、Task submission、pending/completed result materialization、feedback、prompt generation 和 outpaint。                              | 页面只使用 `viewModel.imageWorkflow` 读取 Image Tool 与 outpaint state，并通过 `commands.imageWorkflow` 的 `tools`、`interactions`、`isGenerationAvailable`、`cancelOutpaint` 和 `hideStrengthControl` 发出 intent。Workspace composition root 只注入 graph、execution、Flow Pane、UI adapter 和 feedback dependencies；完整 Session 在 workflow 内一次性组装。                                   | `imageActionIntentRouter`、`generationIntent`、`outpaintIntent`、strength control、`useImageWorkflowExecution`、Task submission sequencing、provider degradation、pending result snapshot、Result Materializer、poll/reattach、media upload/local inpaint fallback 和 node placement 都是 implementation。页面不直接提交 Task、调用 planner 或 materialize result nodes。 |

### Page Boundary

CanvasPage 只负责 route、user identity 和 logout integration；Canvas Screen module 负责 Canvas 的 component binding、overlay placement、keyboard/dismissable UI 和 lightweight DOM refs。Canvas runtime 能力入口只能是 Canvas Facade：

- `useCanvasWorkspaceRuntime()` 返回的 `viewModel` 是 read side；页面不得从 Vue Flow projection 反推 Canvas Board business facts。
- `useCanvasWorkspaceRuntime()` 返回的 `commands` 是 write side；页面触发 graph mutation、board save/load、image workflow、node interaction 和 viewport runtime action 时必须走对应 command。
- 页面不直接 import `application/workspace/internal/*`、`application/board-session` repository、`domain/graph` store instance、execution task-runner 或 result materializer。
- Canvas Screen module 组合 runtime creation、refs 和 lightweight binding，但 chrome placement 必须通过 Canvas Chrome module。Screen 向 Canvas Chrome 传 board/session/tool/account/status state 和 actions，不手写 top/right/bottom/left、inset、z-index、gap、pointer-events 或 hidden animation。

### Canvas Chrome Boundary

Canvas Chrome module 是 Canvas Workspace 的 chrome geometry 和 interaction shell owner。

- 它拥有 chrome region placement：Board header、right action/account group、left tool palette、bottom status/zoom controls 和 minimap anchor。
- 它拥有 interaction shell：pointer-events layering、z-index、dismiss/hidden state、visibility animation、responsive placement、fixed control dimensions、round-control slots 和 group gaps。
- 它接收 CanvasPage 或 workspace runtime 提供的 state/actions，例如 session status、tool selection、account/menu state、save/share/navigation commands、zoom/status actions。它不从 Vue Flow projection 反推业务事实，也不直接执行 graph mutation。
- 它只拥有 minimap 的 chrome anchoring。Minimap internal drawing、math、drag behavior 和 viewport sync 仍属于对应 adapter/runtime；需要 Vue Flow provider 的 minimap DOM 可以通过 `CanvasChromeMiniMap` seam 挂在 Vue Flow subtree 内。
- 它不拥有 graph runtime、Vue Flow adapter、retouch/inpaint runtime、image workflow execution、task submission、result-node materialization 或 application/board-session persistence。

### Vue Flow Adapter Boundary

Vue Flow 是 interaction/rendering Adapter，不是 graph domain。

- `Canvas Node` / `Canvas Connection` / `BoardSnapshot` 是 domain fact；`Flow Node` / `Flow Edge` / native Vue Flow events 是 adapter representation。
- Vue Flow 细节可以存在于 `infrastructure/vue-flow-adapter`、adapter composables 和页面 `<VueFlow>` binding，但不能泄漏到 `domain/graph`、`board/domain` 或页面业务规则。
- 新增 graph domain rule 时，先表达为 Canvas Board/Graph Runtime 语义，再由 adapter 投影到 Vue Flow。

### Execution Projection Contract

`PendingImageExecutionCore` 是 Canvas Task runtime 的唯一 owner。每次 Board open、switch 或 reload 都创建新的 opaque attachment epoch；每个 node execution 使用 lease/CAS transition，旧 attachment 或被 supersede 的 lease 不得修改当前 graph。

- `BoardSnapshot` 只保存 persistence facts，永不保存 Flow Edge `animated`。
- Core state/lease 是当前 Board 唯一 live running fact；presentation、Task cancellation 和 delete lifecycle 必须读取 Core capability，不得维护 running `Ref`/`Set`、透传 running callback，或从 snapshot `outputStatus` 推导 live state。
- Snapshot 中的 `outputStatus: 'running'` 和 `pendingTaskId` 只表示 Hydration 所需的 persisted pending-task fact。只有 Hydration 可以据此建立 reattach lease；reattach driver 只能原子 claim 该 lease 一次，save reconciliation、重复 reattach、presentation 和 delete lifecycle 都不得从 snapshot 反推或复活 running state。
- Whole-graph hydrate、save reconciliation、history restore 和 execution transition 最终都由 Flow runtime 使用当前 Core snapshot 重算所有 image-input edge animation。
- Flow Edge `animated` 是 serializer、history 和 undo/redo contract 之外的 ephemeral projection；history capture/restore 不得记录或恢复其值，semantic graph commit 后必须从当前 Runtime state 重新投影。
- 删除 active execution node 时，Task identity 必须来自当前 Core lease/state；Graph delete 成功后 release lease，并且只取消 Core 返回的 `cancellableTaskIds`；不得通过 snapshot `outputStatus` + `pendingTaskId` 猜测需要取消的 live Task。
- Save 捕获 graph revision；revision 已推进时必须把 upload stabilization 作为有 precondition 的 media patch rebase 到最新 semantic snapshot，不能用旧 snapshot 覆盖当前 groups/media refs。Save reconciliation 只叠加 active lease 拥有、被 serializer 明确排除的 runtime overlay。
- Board attachment 或 execution lease 失效后，旧 async completion 必须被拒绝，不能 materialize、cancel 或投影到新的 Board session。

### `board` Independence

`features/board` 是独立共享的 Board bounded context，也是唯一的 repository/sync fact source。`BoardSnapshot` 是 workspace runtime、board catalog、persistence、preview serialization 和 tests 共同使用的 document contract。

- `features/board/domain` 只拥有 schema、node/connection model、registry、sizing、aspect ratio 和 fusion blend 等 UI/runtime-neutral core。
- `features/board` 不依赖 `features/canvas` 的 concrete runtime、Vue Flow adapter、image tool session 或 page shell。
- `features/canvas` 经 `board` public seam（root barrel 与各层 port）hydrate/export snapshot 并编排 board session；反向依赖会让 shared document contract 被 Workspace implementation 绑死，应避免。

## Rules

- `viewModel.flowPane.nodes` / `viewModel.flowPane.edges` 只允许用于 Vue Flow binding，例如 `CanvasPage.vue` 中的 `const { nodes, edges } = viewModel.flowPane`；不要用它们做业务决策。
- #236 已完成 native Vue Flow connection event binding 的收口；#239 将 bindings 与 programmatic intent 合并为 `commands.flowPane.connectionAdapter`。页面只能通过 adapter 使用 `isValidConnection` / `onConnect`，不得恢复独立 connection command。
- Canvas submodule 协作时只能从 `infrastructure/infrastructure/vue-flow-adapter/index.ts` import connection adapter public contract；不得 deep-import `infrastructure/vue-flow-adapter` implementation。
- Vue Flow native events 和 interaction handlers 放在 `commands.flowPane`，例如 `onNodesChange`、`onEdgesChange`、`connectionAdapter` 和 `nodeDragTransactions`；connection event shape 不作为 direct page command 暴露。
- 页面层访问 Canvas runtime 能力必须通过 `useCanvasWorkspaceRuntime()` 的 `viewModel` / `commands` Facade；新增页面逻辑不要绕过 Facade 直接依赖 runtime internal module。
- CanvasPage 向 Canvas Chrome module 传 state/actions，不新增 route-local chrome geometry class，也不复活 legacy chrome positioning variants。
- Canvas Chrome module 可以拥有 minimap anchoring，但不能接管 minimap internal math、Vue Flow viewport sync 或 graph interaction semantics。
- Graph domain 规则写在 `board/domain` 或 `domain/graph` 语义里；Vue Flow-specific details 只留在 `infrastructure/vue-flow-adapter` 边界。
- #72、#80、#81 已完成。Image Tool Session deepening、Pending Result materialization、Vue Flow deeper adapter cleanup（其余 native event/selection/viewport adapter consolidation）和 performance/flicker optimization 是独立的 post-#74 work，可在各自明确的 Ticket 中继续推进；不得把它们重新并入 #74 或复活旧的 compatibility path。
