<template>
  <PopoverRoot v-model:open="isOpen">
    <PopoverTrigger as-child>
      <button
        type="button"
        :class="[
          adminFramedIconButtonClass,
          active ? '!border-admin-text-1' : '',
          'cursor-pointer',
        ]"
        :aria-label="filterLabel"
        :aria-pressed="active"
      >
        <Filter :class="['size-4', active ? 'text-admin-text-1' : 'text-admin-text-2']" />
      </button>
    </PopoverTrigger>
    <PopoverPortal>
      <PopoverContent
        :class="adminSelectPanelClass"
        :side-offset="ADMIN_SELECT_SIDE_OFFSET"
        align="start"
        @interact-outside="handleInteractOutside"
      >
        <div class="flex flex-col gap-4">
          <div class="border-b border-admin-border pb-2">
            <div class="text-admin-content font-bold text-admin-title-color">{{ title }}</div>
          </div>

          <slot />

          <button
            type="button"
            class="h-9 w-full rounded-lg bg-admin-accent text-white text-admin-body font-medium transition-colors hover:bg-admin-accent-hover active:bg-admin-accent-pressed disabled:opacity-40 disabled:pointer-events-none cursor-pointer shadow-xs"
            :disabled="!active"
            @click="$emit('reset')"
          >
            {{ resetLabel }}
          </button>
        </div>
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Filter } from '@iconoir/vue'
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui'
import { adminFramedIconButtonClass } from '../lib/uiClasses'
import { ADMIN_SELECT_SIDE_OFFSET, adminSelectPanelClass } from '../lib/adminSelectSkin'

withDefaults(
  defineProps<{
    active: boolean
    filterLabel: string
    title?: string
    resetLabel?: string
  }>(),
  {
    title: '筛选',
    resetLabel: '重置筛选',
  },
)

defineEmits<{
  reset: []
}>()

const isOpen = ref(false)

function handleInteractOutside(event: CustomEvent<{ originalEvent: Event }>) {
  const target = (event.detail?.originalEvent?.target ?? event.target) as HTMLElement | null
  // Keep the filter popover open while interacting with select overlays
  // (reka-ui select content and standard listbox/option roles).
  if (target?.closest('[data-reka-select-content], [role=listbox], [role=option]')) {
    event.preventDefault()
  }
}

function toggle() {
  isOpen.value = !isOpen.value
}

function hide() {
  isOpen.value = false
}

defineExpose({
  toggle,
  hide,
})
</script>
