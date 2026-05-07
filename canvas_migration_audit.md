# Canvas → Vue Flow 迁移审计（收尾版）

最后更新：2026-05-07
目标：标记已完成项，保留真实残余，避免后续重复追旧问题。

## 已完成（本轮可关闭）

1. 旧路由/POC 关键引用清理：`picture-flow`、`workspace-flow`、`POC/poc`、`canvasInteractionCounter`、`disabled="true"`、`selected-preview-items="[]"` 在 `apps/web/src` 下已无命中。
2. message 图片 resize 历史语义修正：改为一次交互只在 `mouseup` 且确有变化时入历史；不再逐帧入历史，也不会“无变化却标脏”。
3. 未引用旧实现清理：旧 `useMaskNode.ts` 已移除；`clampMaskRefineAmount` 已迁移到 flow 服务侧实现并复用。

## 当前状态（已迁移但保留关注）

1. Workspace 已以 Vue Flow 为主路径运行，旧 workspace 核心 composables/组件保持删除状态。
2. 节点 UI 基础组件（`Node*`、`useCanvasNodeStyle` 等）仍被新节点层复用，属于“保留资产”，不在清理范围。
3. message 图片 resize 仍保留 8 向与 north/west 锚点计算逻辑；本轮仅修正历史与脏标记行为，不改交互几何算法。

## 残余风险（真实保留）

1. Pointer 输入模型尚未彻底统一：当前仍以鼠标事件为主，触屏/PointerEvent 路径需要后续统一抽象与回归。
2. `WorkspaceScreen.vue` 仍承担较多编排职责，后续仍建议继续拆分（交互、上传生命周期、执行编排等）。
3. 迁移后的手工回归风险仍存在：需持续做关键链路回归（节点编辑、连线、history、board 持久化、生成链路）。

## 后续建议（短清单）

1. 以 pointer 统一为切入点，梳理 resize/drag/select/mask 的输入事件层。
2. 继续拆分 `WorkspaceScreen`，优先抽离高耦合副作用逻辑。
3. 建立最小回归清单并固化到 CI 前检查（至少覆盖 history 与路由入口稳定性）。
