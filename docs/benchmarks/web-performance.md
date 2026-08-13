# Web Performance Baseline

## Bundle baseline

`npm run build` 会在 production Vite build 后执行 `npm run bundle:size:check`。该检查只读取 `apps/web/dist/assets`，守护以下刻意保持粗粒度的预算：

| Asset 类型 | 总量预算 | 单个最大 Asset 预算 |
| ---------- | -------: | ------------------: |
| JavaScript |   3.5 MB |              650 KB |
| CSS        |   350 KB |              180 KB |

2026-08-13 测得的 baseline 约为 3.04 MB JavaScript 和 268 KB CSS；最大的 Asset 分别为 565 KB 的 `CanvasPage` 和 144 KB 的 `index.css`。预算为常规 Feature 开发保留余量，同时让意外的大 Chunk 在 build 阶段可见。

## Canvas interaction benchmark

修改 Canvas rendering 或 interaction 代码时，在本地运行 browser benchmark：

```bash
cd apps/web
npm run benchmark:canvas-interactions -- --json
```

该 benchmark 使用确定性的 150-node fixture、固定 viewport、mock API response，以及逐路径的 frame/mutation budget。通过 `--nodes`、`--paths` 和 `--viewport` 在相同条件下比较两个 revision。报告写入已被 Git 忽略的 `.benchmark-reports/`。

该 benchmark 刻意保持 opt-in。Browser frame timing 对环境敏感，因此不进入每次 required CI；Performance 相关改动可附带 JSON 报告，budget failure 应在相同 runner class 上复查。
