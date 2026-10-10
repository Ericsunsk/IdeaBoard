<template>
  <div class="relative h-full w-full min-w-0">
    <canvas ref="canvasRef" role="img" v-bind="canvasProps" />
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, toRaw, watch, type HTMLAttributes } from 'vue'
import ChartJS, { type ChartType, type Plugin } from 'chart.js/auto'
import type { ChartData, ChartOptions } from '../../core/lib/chartPrimitives'

const props = withDefaults(
  defineProps<{
    type: ChartType | string
    data: ChartData
    options?: ChartOptions | undefined
    plugins?: Plugin[] | undefined
    canvasProps?: HTMLAttributes | undefined
  }>(),
  {
    options: undefined,
    plugins: () => [],
    canvasProps: undefined,
  },
)

const canvasRef = ref<HTMLCanvasElement | null>(null)
let chartInstance: InstanceType<typeof ChartJS> | null = null

function isCanvasReady(): boolean {
  return Boolean(canvasRef.value && canvasRef.value.isConnected)
}

function initChart() {
  if (!isCanvasReady()) return
  if (chartInstance) {
    try {
      chartInstance.destroy()
    } catch {
      // ignore
    }
    chartInstance = null
  }

  try {
    const rawData = toRaw(props.data)
    const rawOptions = props.options ? toRaw(props.options) : {}
    chartInstance = new ChartJS(canvasRef.value!, {
      type: props.type as ChartType,
      data: rawData as never,
      options: rawOptions as never,
      plugins: props.plugins ?? [],
    })
  } catch {
    // ignore creation failure in headless/test environments
  }
}

function updateChart() {
  if (!chartInstance || !isCanvasReady()) {
    if (isCanvasReady()) {
      initChart()
    }
    return
  }
  try {
    chartInstance.data = toRaw(props.data) as never
    if (props.options) {
      chartInstance.options = toRaw(props.options) as never
    }
    chartInstance.update()
  } catch {
    // ignore resize or detach errors in headless/test environments
  }
}

watch(
  () => props.data,
  () => {
    updateChart()
  },
)

watch(
  () => props.options,
  () => {
    if (chartInstance && props.options) {
      try {
        chartInstance.options = toRaw(props.options) as never
        chartInstance.update()
      } catch {
        // ignore
      }
    }
  },
)

watch(
  () => props.type,
  () => {
    initChart()
  },
)

onMounted(() => {
  nextTick(() => {
    initChart()
  })
})

onBeforeUnmount(() => {
  if (chartInstance) {
    try {
      chartInstance.destroy()
    } catch {
      // ignore
    }
    chartInstance = null
  }
})

defineExpose({
  getChart: () => chartInstance,
  reinit: initChart,
  refresh: updateChart,
})
</script>
