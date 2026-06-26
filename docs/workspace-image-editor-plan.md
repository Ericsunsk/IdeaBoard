# Workspace Image Editor Plan

## 背景

当前 image 节点的右键工具主要面向 AI 能力，例如去背景、风格变化、高清放大、扩图、局部重绘。下一步要补一组不依赖 AI 的本地图像编辑能力，例如色相、阈值、灰度、反相等。

目标不是做完整 Photoshop，而是在工作区节点内提供一个轻量、可扩展、PS 心智相近的小型图片编辑面板。

## 目标

- 在 image 节点内提供类似 Photoshop 的编辑布局：左侧工具栏，中间预览画布，右侧属性与图层区域。
- 非 AI 操作默认走本地实时预览，不进入任务队列，不消耗 AI 能力。
- 采用非破坏式编辑心智：原图保持不变，调整以 layer 形式叠加，最终应用为新的 image 节点。
- 先做 UI 与状态模型，再接 Canvas/ImageData 像素处理。

## 非目标

- 第一版不做完整 PS 图层系统。
- 第一版不做图层拖拽排序、混合模式、图层分组、clipping mask。
- 第一版不做服务端图像处理。
- 第一版不把本地图像调整和 AI 局部重绘混成同一个执行链。

## 布局方案

整体采用节点外置 chrome：

```text
左侧：工具栏
中间：图片画布 / 实时预览
右侧上：属性面板 Properties
右侧下：图层面板 Layers
```

### 左侧工具栏

左侧只放工具入口，不放参数。

第一版建议：

- 选择/移动：用于后续选择图层或移动裁剪框，第一版可先禁用。
- 画笔：给蒙版和局部重绘复用。
- 橡皮擦：给蒙版和局部重绘复用。
- 图像调整：色相、饱和度、亮度、对比度。
- 阈值：黑白阈值调整。
- 裁剪：后续能力，第一版可先禁用。

左侧工具切换只影响右侧属性面板内容，不直接提交结果。

### 右侧上：属性面板

属性面板显示当前工具或当前选中 layer 的参数。

选中图像调整 layer：

```text
Properties
色相      slider -180 ~ 180
饱和度    slider -100 ~ 100
亮度      slider -100 ~ 100
对比度    slider -100 ~ 100
```

选中阈值 layer：

```text
Properties
阈值      slider 0 ~ 255
反相      toggle
```

选中蒙版 layer：

```text
Properties
笔刷大小
硬度
蒙版透明度
羽化
```

### 右侧下：图层面板

图层区域提供 PS-like 的核心心智，但第一版保持克制。

第一版 layer 类型：

```text
Layers
- 阈值 1              visible
- 色相/饱和度 1       visible
- 局部重绘蒙版         visible
- 原图                locked
```

第一版支持：

- 选中 layer。
- 显示/隐藏 layer。
- 删除 adjustment layer。
- 原图 layer 锁定，不允许删除。
- 应用当前编辑结果为新 image 节点。

第一版暂不支持：

- 图层拖拽排序。
- blend mode。
- layer opacity。
- group。
- clipping mask。

## 状态模型建议

新增轻量 editor session 状态，避免直接把调整能力塞进局部重绘状态。

```ts
type ImageEditorMode = 'inpaint' | 'adjust'

type ImageEditorTool =
  | 'move'
  | 'brush'
  | 'eraser'
  | 'adjust'
  | 'threshold'
  | 'crop'

type ImageEditorLayer =
  | {
      id: string
      type: 'source'
      name: '原图'
      visible: true
      locked: true
    }
  | {
      id: string
      type: 'adjustment'
      kind: 'hsl' | 'threshold' | 'grayscale' | 'invert'
      name: string
      visible: boolean
      params: Record<string, number | boolean>
    }
  | {
      id: string
      type: 'mask'
      kind: 'inpaint-mask'
      name: string
      visible: boolean
    }
```

## 组件切分建议

先不要大规模重命名现有局部重绘组件。可以新增 image editor shell，复用已有按钮和样式。

建议新增：

- `ImageEditorChrome.vue`：总壳，负责左右区域布局。
- `ImageEditorToolRail.vue`：左侧工具栏。
- `ImageEditorPropertiesPanel.vue`：右侧上属性面板。
- `ImageEditorLayersPanel.vue`：右侧下图层面板。
- `useImageEditorSession.ts`：维护当前 tool、selectedLayerId、layers、参数更新。

继续复用：

- `NodeToolIconButton.vue`
- `node-tool-buttons.css`
- 现有 mask canvas / brush 逻辑可以在进入 inpaint mode 时继续使用。

## 分阶段计划

### Phase 1: UI 壳与图层心智

- Photoshop 按钮进入 `adjust` editor mode。
- 左侧显示工具栏：调整、阈值，其他工具可先禁用。
- 右侧显示 Properties 和 Layers 两个区域。
- 初始化 layer stack：原图 layer + 一个默认 adjustment layer。
- 点击左侧工具时，自动选中或创建对应 adjustment layer。
- 不做实际像素处理，只维护状态并展示控件。

验收：

- image 节点右键点击 Photoshop 按钮后，出现 PS-like 左右面板。
- 左侧工具切换会改变右侧 Properties。
- Layers 面板能选中、显示/隐藏、删除 adjustment layer。

### Phase 2: 本地实时预览

- 使用 Canvas 2D / ImageData 实现 hsl、brightness、contrast、threshold。
- layer visible 影响实时预览。
- 参数 slider 拖动时实时更新预览。
- 原图不被覆盖。

验收：

- 调整参数时中间图片实时变化。
- 隐藏 adjustment layer 后预览恢复对应效果。
- 重置参数可恢复默认预览。

### Phase 3: 应用为新 image 节点

- 将当前预览 canvas 导出为 PNG blob。
- 复用 workspace upload 流程上传图片。
- 在当前 image 节点旁边创建新的 image 节点。
- 原 image 节点保持不变。

验收：

- 点击“应用为新节点”后生成新节点。
- 新节点图片与预览一致。
- 原节点不变。

### Phase 4: 与局部重绘模式协调

- 明确 `inpaint` 与 `adjust` 两种 editor mode。
- `inpaint` 保留 prompt composer 和蒙版提交。
- `adjust` 不显示 prompt composer，只显示本地编辑动作。
- 后续可以让 inpaint mode 也显示 Layers 面板，但不要在第一版强行合并。

验收：

- 局部重绘旧流程不退化。
- Photoshop 本地编辑流程不触发 AI 任务。

## 风险与取舍

- 如果一开始就做完整图层系统，复杂度会过高。第一版只做 source layer 与 adjustment layer。
- 如果直接覆盖原图，工作区节点的分叉心智会变差。默认应生成新节点。
- 如果把局部重绘和本地调整强行塞进一个组件，后续维护会变难。建议共享基础 UI，但 session mode 分离。
- Canvas/ImageData 处理大图可能有性能压力，第一版应限制预览尺寸或做节流。

## 推荐下一步

先做 Phase 1，只实现 UI 壳、工具切换、Properties、Layers 面板和状态模型。像素处理放到 Phase 2。
