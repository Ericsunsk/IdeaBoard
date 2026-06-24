<template>
  <div
    class="canvas-chrome canvas-chrome-status pointer-events-auto absolute right-6 bottom-6 z-[100]"
    :inert="inert"
    @wheel.prevent.stop
  >
    <div
      class="canvas-control-group box-border h-[33px] items-center !gap-[9px] rounded-full bg-white/[0.94] p-1 text-[#202124] [border:var(--canvas-control-border-width)_solid_#d8ddea]"
    >
      <button
        type="button"
        class="canvas-icon-button !m-0 !flex !size-[23px] !cursor-pointer !items-center !justify-center !rounded-full !border-0 !bg-transparent !p-0 !text-[var(--ib-control-icon)] hover:!bg-[#f1f3f9] hover:!text-[var(--ib-control-icon-hover)]"
        aria-label="缩小"
        @click="emit('zoom-out')"
      >
        <Minus class="size-4" />
      </button>
      <span class="min-w-[33px] text-center text-[13px] leading-[1.2] font-normal text-[#202124]">
        {{ zoomLevel }}%
      </span>
      <button
        type="button"
        class="canvas-icon-button !m-0 !flex !size-[23px] !cursor-pointer !items-center !justify-center !rounded-full !border-0 !bg-transparent !p-0 !text-[var(--ib-control-icon)] hover:!bg-[#f1f3f9] hover:!text-[var(--ib-control-icon-hover)]"
        aria-label="放大"
        @click="emit('zoom-in')"
      >
        <Plus class="size-4" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Minus, Plus } from '@iconoir/vue'

defineProps<{
  zoomLevel: number
  inert?: boolean
}>()

const emit = defineEmits<{
  (event: 'zoom-in'): void
  (event: 'zoom-out'): void
}>()
</script>

<style scoped>
@import './styles/canvas-controls.css';
</style>
