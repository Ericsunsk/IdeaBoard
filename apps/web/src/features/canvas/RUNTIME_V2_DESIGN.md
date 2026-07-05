# Canvas Runtime v2 Design Brief

本文是 Canvas performance program 的 runtime 设计 brief。它基于 #65 baseline 和 #66 cleanup 结果，定义后续 #68/#69/#70 的方向；它不是一次性替换 Canvas Workspace 的实施方案。

## 背景证据

Baseline 证据来自 `apps/web/.benchmark-reports/` 的本地 report，汇总已写回 #65。

| Case | Signal |
| ---- | ------ |
| `structure-heavy 500 node-drag` | p95 `116.7ms`，dropped `208`，`nodeArrayReplacements=36` |
| `structure-heavy 1000 node-drag` | p95 `234.1ms`，dropped `479`，`nodeArrayReplacements=36` |
| `structure-heavy 2500 node-drag` | p95 `950ms`，dropped `2005`，`nodeArrayReplacements=36` |
| `structure-heavy 2500 initial-load` | max frame `14933.3ms`，dropped `903`，`nodeArrayReplacements=1668` |
| `structure-heavy 2500 viewport-pan` | p95 低但 max frame `3433.4ms`，dropped `341` |
| `media-heavy 500/1000` | 不比同规模 `structure-heavy` 更糟，media rendering 不是第一瓶颈 |

#66 已移除 `node-drag` commit 的 immediate autosave flush。after benchmark 显示 `autosaveSaveCount` 从 `1` 降为 `0`，但 `nodeArrayReplacements=36` 和 drag p95 仍然很高。因此 Runtime v2 的第一目标是 graph mutation/projection，不是 autosave 微调。

## Goals

- 把 Canvas graph 的事实源从 Vue Flow arrays 迁到 normalized `CanvasGraphStore`。
- 用 `CanvasGraphTransaction` 表达交互边界，让 history/autosave 订阅 transaction commit，而不是响应 raw change floods。
- 让 Vue Flow 成为 Adapter：负责 interaction plane 和 rendering projection，不拥有 domain state。
- 为 `node-drag` 提供第一个 vertical slice，证明真实交互 path 的 p95、dropped frames、replacement counters 有改善。
- 保持第一阶段可回退：旧 Vue Flow runtime path 仍可通过内部开关或 fallback 保留。

## Non-Goals

- 第一阶段不替换 Vue Flow production path。
- 第一阶段不迁移 `CanvasBoardSnapshot` schema。
- 第一阶段不改变 route、Board Session、保存语义或 workflow execution contract。
- 第一阶段不承诺 `10000 nodes + media-heavy` 60fps；`5000/10000` 仍是 torture/ceiling probes。
- 不把 progressive rendering 作为默认体验，除非 benchmark 证明它对实际瓶颈有价值。

## Preserved Contracts

- `CanvasBoardSnapshot` 继续是 persistence schema 和 import/export contract。
- Canvas route 和用户可见行为保持不变：打开 board、拖拽节点、连线、删除、undo/redo、autosave、运行 workflow 的交互语义不变。
- Workflow execution contract 不变：Execution Plan、Task submission、Pending Result Node materialization 仍使用现有 feature contracts。
- `features/canvas-board/domain` 继续拥有 shared snapshot schema/core module。
- Report artifacts 继续只写入 ignored local directory，不能进入 git。

## Current Runtime Shape

当前 runtime 主要由这些 Module 组合：

- `useCanvasWorkspaceRuntime`：组合 board、graph、viewport、task、image tools。
- `useCanvasBoardRuntime` / `useFlowCanvasRuntime`：持有 Vue Flow `nodes`/`edges` arrays，处理 load/save/autosave/history/native changes。
- `useCanvasVueFlowGraphAdapter`：提供 `replaceGraph`、`updateNode`、`applyNodeChanges` 等 command surface。
- `useCanvasGraphRuntime`：处理 node mutation、connections、media inputs、selection、node UI state。
- `useCanvasViewportRuntime`：处理 viewport movement、frame probes 和 viewport persistence。

问题是 Vue Flow controlled arrays 同时承担 domain state、render projection、native interaction patch target。`node-drag` 每次 pointermove 都通过 native node changes 更新 arrays；即使只动一个 node，也会产生 per-step node array replacement。

## Runtime v2 Module Shape

Runtime v2 使用四个主要 Module。每个 Module 的 Interface 是后续实现和测试的 seam。

### `CanvasGraphStore`

`CanvasGraphStore` 是 normalized graph state 的事实源。

Interface responsibilities:

- hydrate from `CanvasBoardSnapshot`
- export to snapshot-compatible graph
- read node/edge by id in O(1)
- expose stable ordered ids for projection
- support transaction begin/commit/cancel
- maintain derived indexes needed by connection rules and media input lookup

Internal state shape:

```ts
type CanvasGraphStoreState = {
  nodesById: Map<string, CanvasGraphNodeRecord>
  edgesById: Map<string, CanvasGraphEdgeRecord>
  nodeOrder: string[]
  edgeOrder: string[]
  indexes: {
    outgoingEdgesByNodeId: Map<string, Set<string>>
    incomingEdgesByNodeId: Map<string, Set<string>>
    nodeIdsByKind: Map<CanvasNodeKind, Set<string>>
  }
}
```

`CanvasGraphStore` must not import Vue Flow. It may store adapter-facing metadata only if the metadata is domain-neutral and serializable back through existing snapshot/export logic.

### `CanvasGraphTransaction`

`CanvasGraphTransaction` groups mutation intent and returns a commit summary.

Interface responsibilities:

- stage node/edge add, update, remove, connect, disconnect, selection and drag updates
- expose cancel without mutating committed store
- commit atomically and return `CanvasGraphTransactionSummary`
- report dirty scope: changed node ids, changed edge ids, removed ids, order changes, viewport changes, history intent and autosave intent

Draft shape:

```ts
type CanvasGraphTransactionSummary = {
  id: string
  reason: 'node-drag' | 'connect' | 'delete' | 'batch-delete' | 'node-update' | 'result-materialize'
  changedNodeIds: string[]
  changedEdgeIds: string[]
  removedNodeIds: string[]
  removedEdgeIds: string[]
  didChangeOrder: boolean
  shouldRecordHistory: boolean
  shouldMarkDirty: boolean
  projectionPatch: CanvasGraphProjectionPatch
}
```

For `node-drag`, drag progress may update transient position projection, but one completed drag should produce at most one history entry and one autosave schedule.

### `VueFlowGraphAdapter`

`VueFlowGraphAdapter` projects `CanvasGraphStore` into Vue Flow and applies narrow patches.

Interface responsibilities:

- initialize Vue Flow arrays from store snapshot
- apply `CanvasGraphProjectionPatch`
- coalesce drag progress updates with `requestAnimationFrame`
- preserve selection and existing user-visible Vue Flow behavior
- expose old path fallback while vertical slices migrate

Projection strategy:

- Preserve object identity for unchanged nodes/edges.
- Replace arrays only when order, visibility, add/remove, or Vue Flow limitations require it.
- For single-node position updates, prefer patching only the changed Flow Node record and avoid re-deriving every node.
- Keep Flow Node UI-only fields separate from Canvas Node domain fields when possible.

The adapter remains allowed to use Vue Flow native APIs. The store must not know that Vue Flow exists.

### `CanvasGraphCommitSubscribers`

History, autosave, viewport persistence, and frame probes subscribe to transaction commit summaries.

Interface responsibilities:

- history receives one summary per logical user action
- autosave schedules once per dirty transaction
- viewport persistence remains separate from node/edge mutation unless transaction reason includes viewport
- benchmark counters attach to summaries rather than raw native changes where possible

This should remove accidental side effects such as raw drag update floods creating repeated history/autosave work.

## Migration Order

1. Build `CanvasGraphStore` and transaction core with no Vue Flow dependency.
2. Add store-level performance tests for `5000` and `10000` node mutation cases without DOM.
3. Wire `node-drag` behind an internal flag as the first vertical slice.
4. Run before/after benchmarks for `structure-heavy 1000 node-drag` and `structure-heavy 5000 node-drag`.
5. If `node-drag` improves, migrate connect/disconnect/delete/batch delete to transactions.
6. Profile Pending Result Node materialization separately before moving result writeback into the transaction layer.
7. Evaluate progressive rendering/degradation only after mutation/projection improvements are measured.
8. Remove the old runtime path only after migrated paths have coverage and fallback is no longer needed.

## Progressive Degradation Hooks

Runtime v2 should expose hooks, not hard-code degradation as the default:

- interaction-active lightweight node mode
- edge lightweight mode, visible-edge mode, or related-edge mode
- media preview lazy/degraded mode
- handle/tool chrome on-demand mounting
- projection budget telemetry for large boards

These hooks belong behind internal flags or adapter options until #71 proves a direction with before/after data.

## Testing And Benchmarks

Required coverage for the next implementation slices:

- `CanvasGraphStore` hydrate/export roundtrip against `CanvasBoardSnapshot`
- add/update/remove/connect/disconnect and batch delete transaction tests
- transaction cancel/commit and dirty summary tests
- node-drag commit and cancel tests
- Vue Flow adapter projection tests for object identity and minimal replacement behavior
- before/after benchmark for `structure-heavy 1000 node-drag`
- torture signal for `structure-heavy 5000 node-drag`, reported as ceiling evidence

Benchmark reports remain local. Issue comments should include compact summaries and exact commands, not committed report JSON.

## Rollback And Fallback

Every migrated path must keep a fallback to the current runtime until the vertical slice is accepted. Acceptable mechanisms:

- internal feature flag
- adapter option selected in `useCanvasWorkspaceRuntime`
- old path preserved for non-migrated operations

Rollback should be a code/config switch back to the old Vue Flow array path, with `CanvasBoardSnapshot` persistence compatibility preserved.

## Open Questions

- Can Vue Flow accept a truly narrow position update without a controlled array replacement, or do we need an adapter-local array replacement with stronger object identity guarantees?
- Should selection remain adapter-owned or become transaction-owned after connect/delete migration?
- Which derived indexes are required for Pending Result Node reattach/materialization?
- What telemetry should become permanent counters versus benchmark-only instrumentation?
