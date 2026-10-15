# Web Performance Baseline

## Bundle baseline

`npm run build` 会在 production Vite build 后执行 `npm run bundle:size:check`。该检查只读取 `apps/web/dist/assets`，守护以下刻意保持粗粒度的预算：

| Asset 类型 | 总量预算 | 单个最大 Asset 预算 |
| ---------- | -------: | ------------------: |
| JavaScript |   3.5 MB |              650 KB |
| CSS        |   350 KB |              205 KB |

2026-10-05 测得的 baseline 约为 4.60 MB JavaScript 和 307 KB CSS；最大的 Asset 分别为 600 KB 的 `CanvasPage` 和 200.16 KB 的 `index.css`。CSS 单个 Asset 预算仅保留约 2.4% 余量，预算调整对应当前 token-layer split；意外的大 Chunk 仍会在 build 阶段失败。

## Canvas interaction benchmark

Canvas logic benchmark 在 Node 中调用 production graph hydrate、snapshot export、node-drag transaction 和完整 Flow projection：

```bash
cd apps/web
npm run benchmark:canvas-interactions -- --json
npm run benchmark:canvas-interactions -- --nodes 1000 --scenarios hydrate,export,node-drag --samples 100 --warmup 10 --seed 42
```

默认 fixture 为 100/500/1000 个 Image、Prompt、Fusion 混合节点，包含连接和 Group；使用固定 seed，并在计时外验证 hydrate/export round trip、mutation 和 projection 数量。每个 scenario 预热 5 次，采样 30 次。`export-and-clone` 单独衡量 export 后额外复制的成本，不代表完整 Save 流程。JSON 报告写入已忽略的 `apps/web/.benchmark-reports/`，记录 fixture hash、revision、working tree/diff hash、runner hash、Node/CPU 环境，以及以 ms 为单位的 mean/p50/p95/max。比较 revision 时使用相同 runner、参数和机器；该数据不包含 browser rendering、网络、图片解码或 FPS，不设置跨机器 timing gate。

Browser runtime measurement 供人工操作时按需开启：

- Development 设置 `VITE_CANVAS_PERF_PROBE=true` 后，pan、zoom、node-drag、load-hydrate 输出真实 `requestAnimationFrame` interval summary。p50/p95 使用最近 4096 个 interval；sampleCount、average 和 max 覆盖完整 session。`droppedFrames` 仅是相对 60 Hz budget 的估算，不能视作 GPU 呈现帧计数。最多保留 256 个 mark，超出数量见 `droppedMarkCount`。
- Console 设置 `window.__canvasInteractionBenchmarkCounters = {}` 可收集 graph command、runtime timing 和 image resource 指标，无需启用 Development log。`runtimeTimingMs` 的 count/totalMs/maxMs 为累计值，p50Ms/p95Ms 使用最近 256 次计时，sampleCount 是实际保留量；每项异步计时与 decode outcome 归属开始时的 collection；替换对象可开始新一轮，旧任务完成不会写入新 collection，删除对象停止收集。
- `gauges` 是最新资源快照；pixel/Blob byte 指标为资源估算，不等同于实际 browser/GPU memory。定位图片生命周期时，分别比较 managed server tier、本地 fallback encode 和 Retouch，不能从 Node logic timing 推断解码/渲染收益。
