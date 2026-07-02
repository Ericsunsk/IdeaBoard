# Canvas Interaction Benchmark

这个目录记录 manual Canvas Interaction Performance benchmark 的运行方式和本轮 `viewport-pan` 优化对比结果。

## 运行

```bash
cd apps/web
npm run benchmark:canvas-interactions
```

脚本会启动本地 Vite dev server，并通过 `playwright-cli` 默认 Chromium browser session 运行固定 `1440x900` desktop viewport。fixture 是可复现的 Medium-Large Canvas Board：150 个 Canvas Nodes、240 条 edges，包含 image、prompt、fusion 三类节点。

## 输出

脚本向 stdout 输出 JSON report，覆盖：

- `node-drag`
- `viewport-pan`
- `viewport-zoom`

每个 path 包含 `p95FrameMs`、`droppedFrames`、`durationMs`、`avgFrameMs`、`maxFrameMs`、`sampleCount`、`mutationCount`、`eventCount`，并在适用时包含 `positionChanges` 或 `viewportChanges`。

保存一次 run：

```bash
npm run benchmark:canvas-interactions > benchmarks/canvas-interactions/latest.json
```

比较 optimized path：

```bash
node -e "const fs=require('fs'); for (const f of ['viewport-pan-before.json','viewport-pan-after.json']) { const r=JSON.parse(fs.readFileSync('benchmarks/canvas-interactions/'+f,'utf8')); console.log(f, r.metrics) }"
```

## 本轮优化结果

`viewport-pan` 是 baseline 中最差的 Primary Canvas Interaction Path。只对该 path 加了 pan 期间的 render/hit-test 轻量化。

- Before: `p95FrameMs` 33.3, `droppedFrames` 18, `durationMs` 1272.6
- After: `p95FrameMs` 17.7, `droppedFrames` 16, `durationMs` 1236.5
