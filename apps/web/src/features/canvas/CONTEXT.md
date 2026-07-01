# Canvas Workspace

Canvas Workspace 是用户编辑 Canvas Board、编排节点、运行图像任务并就地编辑图像结果的上下文。

## Language

**Canvas Workspace**:
一个 Board Session 的交互工作区，包含 graph interaction、overlay、image tools 和 execution feedback。

**Canvas Board**:
持久化的 graph document，由节点、连接、viewport state 和 board metadata 组成。
_Avoid_: Vue Flow graph, canvas state

**Board Session**:
单个 Canvas Board 的打开、保存、autosave、preview 和导航生命周期。

**Vue Flow Pane**:
Vue Flow 提供的 graph interaction plane，负责节点布局、viewport movement、selection 和 native graph events。
_Avoid_: Canvas, board

**HTMLCanvasElement**:
浏览器 raster drawing surface，用于 mask preview、mask commit、histogram 和 rendered image adjustment 等像素操作。
_Avoid_: Canvas Board, Canvas Workspace

**Canvas Node**:
Canvas Board 中的 domain node，例如 prompt node、image node 或 fusion node。
_Avoid_: Flow node

**Flow Node**:
Canvas Workspace 挂载期间，Canvas Node 在 Vue Flow 中的 runtime representation。
_Avoid_: Canvas Node

**Task**:
Canvas Workspace 提交到后端的 image generation 或 image editing job。
_Avoid_: Execution plan, pending node

**Execution Plan**:
把 Canvas Node 转成一个或多个 Task 的准备结果，包含 source images、action mode、sizing、prompt 和 mask inputs。
_Avoid_: Task

**Pending Result Node**:
代表已请求但尚未完成的 Task output 的临时 Canvas Node。
_Avoid_: Task, execution plan

**Image Workflow Execution**:
准备 image action request、提交 Task，并 materialize pending 或 completed image result 的能力。

**Image Editing**:
image node 的非破坏性 adjustment mode，包含 adjustment layers 和 rendered previews。
_Avoid_: Mask editing

**Image Mask Editing**:
image node 的 mask authoring mode，包含 brush state、mask preview 和 committed mask data。

**Mask Draft**:
Image Mask Editing 激活时的 in-progress mask state。
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
一个 Canvas Workspace 内对 Image Editing、Image Mask Editing、chrome state、node bindings 和 workflow actions 的 active coordination。
_Avoid_: Image editing domain, mask domain
