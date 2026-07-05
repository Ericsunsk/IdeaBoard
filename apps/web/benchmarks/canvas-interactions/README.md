# Canvas Interaction Benchmark

这个目录记录 manual Canvas Interaction Performance benchmark 的运行方式和本轮 `viewport-pan` 优化对比结果。

## 运行

```bash
cd apps/web
npm run benchmark:canvas-interactions
```

脚本会启动本地 Vite dev server，并通过 `playwright-cli` 默认 Chromium browser session 运行固定 `1440x900` desktop viewport。默认 fixture 是可复现的 `structure-heavy` Canvas Board：约 150 个 Canvas Nodes、240 条 edges，包含 image、prompt、fusion 三类节点。

常用参数：

```bash
npm run benchmark:canvas-interactions -- --nodes 500 --paths node-drag
npm run benchmark:canvas-interactions -- --paths viewport-pan,viewport-zoom --json
```

支持参数：

- `--fixture structure-heavy`
- `--nodes <total-node-count>`：总节点数，不是每类节点数。
- `--paths node-drag,viewport-pan,viewport-zoom`
- `--output <report-directory>`
- `--json`：向 stdout 输出完整 JSON report。

实用基准优先使用 `150`、`500`、`1000`、`2500`。`5000` 和 `10000` 只作为手动 torture / ceiling probe，不作为默认命令或 pass/fail gate。

## 输出

脚本默认向 stdout 输出 compact summary，并把完整 JSON report 写入：

```text
apps/web/.benchmark-reports/canvas-interactions/
```

Report artifacts 是本地生成物，不能进入 git。需要完整 JSON stdout 时使用 `--json`。

脚本根据内置 budgets 给出 `verdict.status`：

- `node-drag`
- `viewport-pan`
- `viewport-zoom`

每个 path 包含 `p95FrameMs`、`droppedFrames`、`durationMs`、`avgFrameMs`、`maxFrameMs`、`sampleCount`、`mutationCount`、`eventCount`，并在适用时包含 `positionChanges` 或 `viewportChanges`。

每个 path 还包含 `counters`，用于区分 scripting hot path 与 rendering-only cost：

- `nodeArrayReplacements` / `edgeArrayReplacements`
- `nativeNodeChangeBatches` / `nativeEdgeChangeBatches`
- `graphCommandCount` 和 `graphCommands`
- `historyCaptureCount` / `historyPushCount`
- `autosaveScheduleCount` / `autosaveSaveCount`

如果未观察到 node drag movement 或 viewport movement，脚本会直接失败。若任一 path 超出 `budgets` 中配置的 `p95FrameMs`、DOM `mutationCount`、array replacement、history/autosave 或 native batch 阈值，`verdict.status` 为 `fail`，命令以 exit code `1` 结束。

比较 optimized path：

```bash
node -e "const fs=require('fs'); for (const f of ['viewport-pan-before.json','viewport-pan-after.json']) { const r=JSON.parse(fs.readFileSync('benchmarks/canvas-interactions/'+f,'utf8')); console.log(f, r.metrics) }"
```

## 本轮优化结果

`viewport-pan` 是 baseline 中最差的 Primary Canvas Interaction Path。只对该 path 加了 pan 期间的 render/hit-test 轻量化。

- Before: `p95FrameMs` 33.3, `droppedFrames` 18, `durationMs` 1272.6
- After: `p95FrameMs` 17.7, `droppedFrames` 16, `durationMs` 1236.5
