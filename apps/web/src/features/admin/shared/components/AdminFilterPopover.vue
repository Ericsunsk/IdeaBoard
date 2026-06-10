<template>
  <Button
    text
    :class="[adminToolbarIconButtonClass, active ? '!border-admin-text-1' : '']"
    :aria-label="filterLabel"
    :aria-pressed="active"
    @click="toggle"
  >
    <template #icon>
      <Filter :class="['size-4', active ? 'text-admin-text-1' : 'text-admin-text-2']" />
    </template>
  </Button>
  <Popover ref="popoverRef">
    <div :class="adminFilterPopoverClass">
      <div :class="adminPopoverSectionHeaderClass">
        <div :class="adminPopoverTitleClass">{{ title }}</div>
      </div>

      <slot />

      <Button
        :label="resetLabel"
        size="small"
        severity="contrast"
        :disabled="!active"
        @click="$emit('reset')"
      />
    </div>
  </Popover>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import Button from 'primevue/button'
import Popover from 'primevue/popover'
import { Filter } from '@iconoir/vue'
import {
  adminFilterPopoverClass,
  adminPopoverSectionHeaderClass,
  adminPopoverTitleClass,
  adminToolbarIconButtonClass,
} from '../lib/adminUiClasses'

type PrimePopupPanel = {
  toggle: (event: Event) => void
  hide?: () => void
}

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

const popoverRef = ref<PrimePopupPanel | null>(null)

function toggle(event: Event) {
  popoverRef.value?.toggle(event)
}

function hide() {
  popoverRef.value?.hide?.()
}

defineExpose({
  toggle,
  hide,
})
</script>
