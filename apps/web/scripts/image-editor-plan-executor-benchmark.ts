/// <reference lib="dom" />

import { performance } from 'node:perf_hooks'
import { createCpuImageAdjustmentPlanExecutor } from '../src/features/workspace/image-editor/runtime/cpuPlanExecutor'
import type {
  ImageAdjustmentPlan,
  ImageAdjustmentPlanStep,
} from '../src/features/workspace/image-editor/utils/imageAdjustmentEngine'
import { compileImageAdjustmentPlan } from '../src/features/workspace/image-editor/utils/imageAdjustmentEngine'
import type { ImageEditorAdjustmentLayer } from '../src/features/workspace/image-editor/types'

type ImageDataCtor = new (data: Uint8ClampedArray, width: number, height: number) => ImageData

type BenchmarkResult = {
  label: string
  iterations: number
  totalMs: number
  perIterationMs: number
}

const IMAGE_WIDTH = 512
const IMAGE_HEIGHT = 512
const COMPILE_ITERATIONS = 20_000
const EXECUTE_ITERATIONS = 24
const WARMUP_EXECUTE_ITERATIONS = 3

installNodeImageDataPolyfill()

async function main() {
  const layers = createBenchmarkLayers()
  const plan = compileImageAdjustmentPlan(layers)
  assertBenchmarkPlan(plan)

  const sourcePixels = createSourcePixels(IMAGE_WIDTH, IMAGE_HEIGHT)
  await warmUp(layers, plan, sourcePixels)

  const compileResult = measure('compile plan', COMPILE_ITERATIONS, () => {
    compileImageAdjustmentPlan(layers)
  })
  const executeResult = await measureAsync('cpu executor', EXECUTE_ITERATIONS, async () => {
    await executePlan(plan, sourcePixels)
  })
  const checksum = checksumImageData(await executePlan(plan, sourcePixels))

  console.log('image-editor plan executor benchmark')
  console.log(
    `image: ${IMAGE_WIDTH}x${IMAGE_HEIGHT} (${formatNumber(IMAGE_WIDTH * IMAGE_HEIGHT)} px)`,
  )
  console.log(`plan: ${plan.length} steps (${summarizePlan(plan)})`)
  console.log(formatResult(compileResult))
  console.log(`${formatResult(executeResult)} (${formatThroughput(executeResult)} MP/s)`)
  console.log(`checksum: ${checksum}`)
}

async function warmUp(
  layers: readonly ImageEditorAdjustmentLayer[],
  plan: ImageAdjustmentPlan,
  sourcePixels: Uint8ClampedArray,
) {
  for (let index = 0; index < 1_000; index += 1) {
    compileImageAdjustmentPlan(layers)
  }

  for (let index = 0; index < WARMUP_EXECUTE_ITERATIONS; index += 1) {
    await executePlan(plan, sourcePixels)
  }
}

function measure(label: string, iterations: number, run: (index: number) => void): BenchmarkResult {
  const start = performance.now()
  for (let index = 0; index < iterations; index += 1) {
    run(index)
  }
  const totalMs = performance.now() - start
  return {
    label,
    iterations,
    totalMs,
    perIterationMs: totalMs / iterations,
  }
}

async function measureAsync(
  label: string,
  iterations: number,
  run: (index: number) => Promise<void>,
): Promise<BenchmarkResult> {
  const start = performance.now()
  for (let index = 0; index < iterations; index += 1) {
    await run(index)
  }
  const totalMs = performance.now() - start
  return {
    label,
    iterations,
    totalMs,
    perIterationMs: totalMs / iterations,
  }
}

async function executePlan(
  plan: ImageAdjustmentPlan,
  sourcePixels: Uint8ClampedArray,
): Promise<ImageData> {
  const executor = createCpuImageAdjustmentPlanExecutor()
  return executor.execute({
    imageData: createImageDataCopy(sourcePixels, IMAGE_WIDTH, IMAGE_HEIGHT),
    plan,
    signal: new AbortController().signal,
    onProgress: () => undefined,
  }) as ImageData
}

function createBenchmarkLayers(): readonly ImageEditorAdjustmentLayer[] {
  return [
    {
      id: 'benchmark:hsl:1',
      type: 'adjustment',
      kind: 'hsl',
      name: 'Benchmark HSL',
      visible: true,
      params: {
        hue: 18,
        saturation: 22,
        brightness: -8,
        contrast: 12,
      },
    },
    {
      id: 'benchmark:curves',
      type: 'adjustment',
      kind: 'curves',
      name: 'Benchmark Curves',
      visible: true,
      params: {
        channels: {
          rgb: [{ id: 'point:benchmark:midtone', input: 42, output: 58 }],
        },
      },
    },
    {
      id: 'benchmark:levels',
      type: 'adjustment',
      kind: 'levels',
      name: 'Benchmark Levels',
      visible: true,
      params: {
        shadows: 8,
        midtones: 45,
        highlights: 94,
      },
    },
    {
      id: 'benchmark:invert',
      type: 'adjustment',
      kind: 'invert',
      name: 'Benchmark Invert',
      visible: true,
      params: {
        enabled: true,
      },
    },
    {
      id: 'benchmark:threshold',
      type: 'adjustment',
      kind: 'threshold',
      name: 'Benchmark Threshold',
      visible: true,
      params: {
        threshold: 124,
        invert: false,
      },
    },
    {
      id: 'benchmark:hsl:2',
      type: 'adjustment',
      kind: 'hsl',
      name: 'Benchmark HSL Finish',
      visible: true,
      params: {
        hue: -12,
        saturation: -16,
        brightness: 6,
        contrast: -6,
      },
    },
  ]
}

function assertBenchmarkPlan(plan: ImageAdjustmentPlan) {
  if (plan.length === 0) {
    throw new Error('benchmark plan unexpectedly compiled to zero steps')
  }
  if (
    !plan.some((step) => step.kind === 'channel-lut') ||
    !plan.some((step) => step.kind === 'hsl')
  ) {
    throw new Error(`benchmark plan is too narrow: ${summarizePlan(plan)}`)
  }
}

function createSourcePixels(width: number, height: number) {
  const pixels = new Uint8ClampedArray(width * height * 4)
  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const index = (y * width + x) * 4
      pixels[index] = (x * 17 + y * 3) % 256
      pixels[index + 1] = (x * 5 + y * 19) % 256
      pixels[index + 2] = (x * 11 + y * 7) % 256
      pixels[index + 3] = 255
    }
  }
  return pixels
}

function createImageDataCopy(sourcePixels: Uint8ClampedArray, width: number, height: number) {
  const ImageDataConstructor = globalThis.ImageData as ImageDataCtor
  return new ImageDataConstructor(new Uint8ClampedArray(sourcePixels), width, height)
}

function checksumImageData(imageData: ImageData) {
  let checksum = 0
  for (let index = 0; index < imageData.data.length; index += 1) {
    checksum = (checksum + imageData.data[index] * ((index % 251) + 1)) % 1_000_000_007
  }
  return checksum
}

function summarizePlan(plan: ImageAdjustmentPlan) {
  const counts = new Map<ImageAdjustmentPlanStep['kind'], number>()
  for (const step of plan) {
    counts.set(step.kind, (counts.get(step.kind) ?? 0) + 1)
  }
  return Array.from(counts.entries())
    .map(([kind, count]) => `${kind} x${count}`)
    .join(', ')
}

function formatResult(result: BenchmarkResult) {
  return [
    `${result.label}:`,
    `${formatNumber(result.iterations)} runs,`,
    `${formatMs(result.totalMs)} total,`,
    `${formatMs(result.perIterationMs)} / run`,
  ].join(' ')
}

function formatThroughput(result: BenchmarkResult) {
  const pixelsProcessed = IMAGE_WIDTH * IMAGE_HEIGHT * result.iterations
  const megapixelsPerSecond = pixelsProcessed / 1_000_000 / (result.totalMs / 1_000)
  return megapixelsPerSecond.toFixed(2)
}

function formatMs(value: number) {
  return `${value.toFixed(2)} ms`
}

function formatNumber(value: number) {
  return new Intl.NumberFormat('en-US').format(value)
}

function installNodeImageDataPolyfill() {
  if (typeof globalThis.ImageData === 'function') return

  class NodeImageData {
    readonly data: Uint8ClampedArray
    readonly width: number
    readonly height: number

    constructor(data: Uint8ClampedArray, width: number, height: number) {
      if (data.length !== width * height * 4) {
        throw new Error(`Invalid ImageData size: got ${data.length} bytes for ${width}x${height}`)
      }
      this.data = data
      this.width = width
      this.height = height
    }
  }

  Object.defineProperty(globalThis, 'ImageData', {
    configurable: true,
    value: NodeImageData,
    writable: true,
  })
}

void main().catch((error) => {
  console.error(error)
  process.exitCode = 1
})
