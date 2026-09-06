# Canvas Graph Runtime Design Brief

本文记录 Canvas performance program 的当前 graph runtime 主路径。Graph Runtime 已是 Canvas Workspace 的 canonical graph mutation path，不再保留迁移期 switch、旧 connection id path、临时降级渲染 path 或旧 graph mutation path。

## Background Evidence

Baseline 证据来自 `apps/web/.benchmark-reports/` 的本地 report，汇总已写回 #65。

| Case                                | Signal                                                          |
| ----------------------------------- | --------------------------------------------------------------- |
| `structure-heavy 500 node-drag`     | p95 `116.7ms`，dropped `208`，`nodeArrayReplacements=36`        |
| `structure-heavy 1000 node-drag`    | p95 `234.1ms`，dropped `479`，`nodeArrayReplacements=36`        |
| `structure-heavy 2500 node-drag`    | p95 `950ms`，dropped `2005`，`nodeArrayReplacements=36`         |
| `structure-heavy 2500 initial-load` | max frame `132.5ms`，dropped `12`，`longTaskMax=131ms`          |
| `structure-heavy 2500 viewport-pan` | p95 低但 max frame `3433.4ms`，dropped `341`                    |
| `media-heavy 500/1000`              | 不比同规模 `structure-heavy` 更糟，media rendering 不是第一瓶颈 |

#66 已移除 `node-drag` commit 的 immediate autosave flush。after benchmark 显示 `autosaveSaveCount` 从 `1` 降为 `0`，但 `nodeArrayReplacements=36` 和 drag p95 仍然很高。因此 Graph Runtime 的核心目标是 graph mutation/projection，而不是 autosave 微调。

## Goals

- 用 normalized `CanvasGraphStore` 作为 Canvas graph mutation 的 fact source。
- 用 `CanvasGraphTransaction` 表达 interaction boundary，让 history/autosave 订阅 transaction commit，而不是响应 raw change floods。
- 让 Vue Flow 成为 interaction plane 和 rendering projection adapter，不拥有 domain mutation semantics。
- 让 `node-drag`、connect、disconnect、delete 和 batch delete 都走 canonical transaction path。
- 对不可写入 canonical graph 的 corrupt/non-canonical data 显式 warning，要求刷新或重新 hydrate，不再静默回落到旧 mutation path。

## Non-Goals

- 不迁移 `CanvasBoardSnapshot` schema。
- 不改变 route 或持久化 schema；Board Session 与 workflow execution 通过 attachment/lease contract 协作。
- 不承诺 `10000 nodes + media-heavy` 60fps；`5000/10000` 是 torture/ceiling probes。
- 不把 progressive/degraded rendering 作为默认体验。

## Preserved Contracts

- `CanvasBoardSnapshot` 继续是 persistence schema 和 import/export contract。
- Canvas route 保持不变；支持打开 board、拖拽节点、创建/删除连线、undo/redo、autosave 和运行 workflow。已创建连线的 endpoints 不可变，调整位置必须删除后重新创建。
- Execution Plan、Task submission 与 Pending Result materialization 继续使用 feature boundaries；runtime state 由单一 `PendingImageExecutionCore` attachment/lease seam 拥有。
- `features/canvas-board/domain` 继续拥有 shared snapshot schema/core module。
- Benchmark report artifacts 继续只写入 ignored local directory，不能进入 git。

## Module Shape

### `CanvasGraphStore`

`CanvasGraphStore` 是 normalized graph state 的 fact source。

Responsibilities:

- hydrate from `CanvasBoardSnapshot`
- export to snapshot-compatible graph
- read node/edge by id in O(1)
- expose stable ordered ids for projection
- support transaction begin/commit/cancel
- maintain derived indexes needed by connection rules and media input lookup

`CanvasGraphStore` must not import Vue Flow. It may store adapter-facing metadata only when the metadata is domain-neutral and serializable back through existing snapshot/export logic.

### `CanvasGraphTransaction`

`CanvasGraphTransaction` groups mutation intent and returns a commit summary.

Responsibilities:

- stage node add/update/remove、edge add/remove、connect/disconnect、selection 和 drag updates
- expose cancel without mutating committed store
- commit atomically and return `CanvasGraphTransactionSummary`
- report dirty scope: changed node ids, changed edge ids, removed ids, order changes, history intent and autosave intent

For `node-drag`, Vue Flow owns transient renderer measurements and interaction fields. Canvas may
mirror the current position and selection into the controlled model, but never feeds renderer-only
geometry back into it. At drag stop, the Graph Runtime commits the coalesced final position
through the renderer adapter. One completed drag produces at most one history entry and one
autosave schedule.

### `CanvasVueFlowGraphAdapter`

`CanvasVueFlowGraphAdapter` applies Graph Runtime projections to Vue Flow controlled arrays.

Responsibilities:

- initialize Vue Flow arrays from board snapshot projection
- apply graph adapter commands with command side effects
- keep Vue Flow geometry, handles and transient interaction fields out of the controlled node
  model; apply committed canonical changes through the Vue Flow renderer adapter
- preserve selection and existing user-visible Vue Flow behavior
- keep Vue Flow-specific node/edge shape outside `CanvasGraphStore`

Projection strategy:

- Preserve object identity for unchanged nodes/edges.
- Replace arrays only when order, visibility, add/remove, or Vue Flow limitations require it.
- For single-node position updates, patch only the changed Flow Node record when practical.
- Keep Flow Node UI-only fields separate from Canvas Node domain fields when possible.
- Keep the complete Graph in the Vue Flow controlled arrays. Viewport movement only changes the
  transform; it must not replace the node/edge arrays because doing so makes connected edges
  disappear while zooming and also makes MiniMap represent only a partial board.

### `PendingImageExecutionCore`

Execution Runtime owns transient Task lifecycle projection, not persistence facts.

Responsibilities:

- create a new opaque attachment epoch for every Board open, switch, or reload
- issue per-node execution leases and reject stale attachment/lease transitions with CAS semantics
- expose the current attachment's node execution states as the only image-input animation source
- establish running leases from persisted pending tasks only during hydration
- allow the reattach driver to claim each hydration lease once, but never recreate it
- retain serializer-omitted nodes and edges only while an active execution lease owns them
- keep animation out of `CanvasBoardSnapshot` and graph dirty/history accounting

Flow runtime applies execution projection synchronously after semantic graph reconciliation and before replacing Vue Flow arrays. Save reconciliation is revision-gated; upload stabilization rebases only media fields whose captured values are still current, preserves newer groups/media refs, and never treats saved `outputStatus` as live execution state.

Clean-break constraints:

- `PendingImageExecutionCore` state/lease is the only live running fact. Presentation, Task cancellation, and delete lifecycle consume Core capability directly; they must not own shadow running `Ref`/`Set` state or relay running state through optional callbacks.
- Snapshot `outputStatus: 'running'` and `pendingTaskId` are hydration facts only. Outside Hydration, code must not infer live execution, animation, or deletion cancellation from them.
- Flow Edge `animated` is an ephemeral Runtime projection. Snapshot serialization, history capture, and undo/redo restore exclude it; every semantic graph replacement reprojects it from the current Runtime state.
- Deleting an active execution node resolves Task identity from the current Runtime lease/state, never from snapshot running fields.

### Commit Side Effects

History, autosave, viewport persistence, and frame probes subscribe to graph command side effects derived from transaction summaries.

Responsibilities:

- history receives one entry per logical user action
- autosave schedules once per dirty transaction
- viewport persistence remains separate from node/edge mutation unless explicitly marked dirty
- benchmark counters attach to graph commands and interaction batches

## Canonical Paths

- `node-drag`: queue native position/select changes, project per frame, commit one `node-drag` transaction.
- connect: normalize connection, compute canonical `connection-*` id, commit one `connect` transaction.
- native disconnect/delete/batch delete: coalesce native remove changes, commit one `disconnect` / `delete` / `batch-delete` transaction.
- node delete command: remove canonical graph node and connected edges via one `delete` transaction.

Connection endpoints are immutable. Repositioning a connection is modeled as an explicit delete followed by a new connect, producing independent transaction and history boundaries.

Non-canonical node ids, non-canonical edge ids, or corrupt edge snapshots are not mutated through a legacy path. The UI must surface a warning and leave the current graph untouched.

## Testing And Benchmarks

Required coverage:

- `CanvasGraphStore` hydrate/export roundtrip against `CanvasBoardSnapshot`
- node add/update/remove、edge add/remove、connect/disconnect 和 batch delete transaction tests
- transaction cancel/commit and dirty summary tests
- node-drag commit and cancel tests
- connection tests using canonical connection ids
- native disconnect/delete/batch delete tests asserting one undoable transaction
- before/after benchmark for `structure-heavy 1000 node-drag`
- torture signal for `structure-heavy 5000 node-drag`, reported as ceiling evidence

Benchmark reports remain local. Issue comments should include compact summaries and exact commands, not committed report JSON.

## Projection Boundary

The canonical Graph and the Vue Flow controlled arrays contain the same complete
node/edge set. Vue Flow's `onlyRenderVisibleElements` is disabled deliberately:
viewport clipping changes the controlled node set during zoom and pan, which can
detach edges and makes MiniMap bounds unstable. Performance work therefore stays
on Graph transactions, stable object identity, and incremental adapter updates;
viewport changes never trigger graph projection.

## Future Optimization Candidates

Only add these after benchmark evidence shows they address the measured bottleneck:

- edge-level incremental projection (only after Vue Flow can apply add/remove
  mutations without exposing transient dangling endpoints)
- edge projection budget / related-edge mode
- media preview lazy/degraded mode
- handle/tool chrome on-demand mounting
- result materialization through Graph Runtime
- permanent telemetry counters beyond benchmark-only instrumentation
