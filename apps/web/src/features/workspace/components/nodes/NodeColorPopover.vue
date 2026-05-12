<template>
  <div
    class="absolute top-[calc(100%+8px)] left-0 z-24 flex w-[210px] flex-col gap-2 rounded-xl border border-[#dce2f1] bg-white p-2.5"
    @mousedown.stop
    @click.stop
  >
    <div class="flex min-w-0 items-center gap-2">
      <div
        class="h-7 w-7 shrink-0 rounded-full border border-[rgba(0,0,0,0.08)]"
        :style="{ background: hex }"
      ></div>
      <input
        :value="hex"
        class="box-border h-7 min-w-0 flex-1 rounded-lg border border-[#d6dcec] bg-[#f9fbff] px-2 font-mono text-xs text-[#425067] uppercase outline-none focus:border-[#9278e6] focus:bg-white"
        type="text"
        maxlength="7"
        @input="emit('hex-input', $event)"
        @blur="emit('hex-commit')"
        @keydown.enter.prevent="emit('hex-commit')"
      />
    </div>
    <label class="grid grid-cols-[72px_1fr] items-center gap-2">
      <span class="text-[11px] font-semibold text-[#6b7688]">Hue</span>
      <input
        :value="hue"
        class="node-color-popover-range hue h-3.5 w-full cursor-pointer appearance-none bg-transparent"
        type="range"
        min="0"
        max="360"
        @input="emit('hue-input', toNumber(($event.target as HTMLInputElement).value, hue))"
      />
    </label>
    <label class="grid grid-cols-[72px_1fr] items-center gap-2">
      <span class="text-[11px] font-semibold text-[#6b7688]">Saturation</span>
      <input
        :value="saturation"
        class="node-color-popover-range h-3.5 w-full cursor-pointer appearance-none bg-transparent"
        type="range"
        min="0"
        max="100"
        @input="
          emit('saturation-input', toNumber(($event.target as HTMLInputElement).value, saturation))
        "
      />
    </label>
    <label class="grid grid-cols-[72px_1fr] items-center gap-2">
      <span class="text-[11px] font-semibold text-[#6b7688]">Brightness</span>
      <input
        :value="brightness"
        class="node-color-popover-range h-3.5 w-full cursor-pointer appearance-none bg-transparent"
        type="range"
        min="0"
        max="100"
        @input="
          emit('brightness-input', toNumber(($event.target as HTMLInputElement).value, brightness))
        "
      />
    </label>
    <div
      ref="spectrumRefEl"
      class="relative h-[94px] w-full cursor-crosshair overflow-hidden rounded-[10px] border border-[#d6dcec]"
      :style="spectrumStyle"
      @pointerdown.stop.prevent="emit('spectrum-pointerdown', $event)"
    >
      <span
        class="pointer-events-none absolute h-3 w-3 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow-[0_0_0_1px_rgba(25,31,45,0.48)]"
        :style="cursorStyle"
      ></span>
    </div>
    <div class="grid grid-cols-8 gap-1.5 pt-0.5">
      <button
        v-for="swatch in quickSwatches"
        :key="swatch"
        type="button"
        class="h-4 w-4 rounded-full border border-[rgba(0,0,0,0.08)]"
        :style="{ background: swatch }"
        @click="emit('select-swatch', swatch)"
      ></button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { toNumber } from './numberUtils'

defineProps<{
  hex: string
  hue: number
  saturation: number
  brightness: number
  spectrumStyle: Record<string, string>
  cursorStyle: Record<string, string>
  quickSwatches: string[]
}>()

const emit = defineEmits<{
  (e: 'hex-input', event: Event): void
  (e: 'hex-commit'): void
  (e: 'hue-input', value: number): void
  (e: 'saturation-input', value: number): void
  (e: 'brightness-input', value: number): void
  (e: 'spectrum-pointerdown', event: PointerEvent): void
  (e: 'select-swatch', value: string): void
  (e: 'set-spectrum-ref', value: HTMLElement | null): void
}>()

const spectrumRefEl = ref<HTMLElement | null>(null)

watch(
  spectrumRefEl,
  (value) => {
    emit('set-spectrum-ref', value ?? null)
  },
  { immediate: true },
)
</script>

<style scoped>
.node-color-popover-range.hue {
  --range-accent: #a855f7;
}

.node-color-popover-range:not(.hue) {
  --range-accent: #7a6bd5;
}

.node-color-popover-range::-webkit-slider-runnable-track {
  height: 6px;
  border-radius: 999px;
  background: linear-gradient(
    90deg,
    var(--range-accent) 0%,
    color-mix(in srgb, var(--range-accent) 56%, #ffffff) 100%
  );
}

.node-color-popover-range::-webkit-slider-thumb {
  appearance: none;
  width: 11px;
  height: 11px;
  border-radius: 999px;
  margin-top: -2.5px;
  background: var(--range-accent);
}

.node-color-popover-range::-webkit-slider-thumb:active {
  background: var(--range-accent);
}

.node-color-popover-range::-moz-range-track {
  height: 6px;
  border: none;
  border-radius: 999px;
  background: linear-gradient(
    90deg,
    var(--range-accent) 0%,
    color-mix(in srgb, var(--range-accent) 56%, #ffffff) 100%
  );
}

.node-color-popover-range::-moz-range-thumb {
  width: 11px;
  height: 11px;
  border: none;
  border-radius: 999px;
  background: var(--range-accent);
}

.node-color-popover-range:active,
.node-color-popover-range:focus,
.node-color-popover-range::-webkit-slider-runnable-track:active,
.node-color-popover-range::-moz-range-track:active,
.node-color-popover-range::-moz-range-thumb:active {
  opacity: 1;
  filter: none;
}
</style>
