<template>
  <Popover ref="popoverRef" :pt="popoverPt" @hide="emit('hide')">
    <div class="flex min-w-[176px] flex-col gap-1 p-1.5">
      <template v-for="item in items" :key="item.key">
        <div v-if="item.divided" class="my-1 h-px bg-admin-border-soft" aria-hidden="true" />
        <Button
          :label="item.label"
          text
          :severity="item.danger ? 'danger' : 'secondary'"
          :disabled="item.disabled"
          fluid
          class="justify-start rounded-md px-3 py-2.5 text-admin-meta font-[650]"
          @click="emit('select', item.key)"
        />
      </template>
    </div>
  </Popover>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import Button from 'primevue/button'
import Popover from 'primevue/popover'
import { createAdminPopoverPt } from '../lib/adminOverlayPt'

export type AdminRowActionItem = {
  key: string
  label: string
  disabled?: boolean
  divided?: boolean
  danger?: boolean
}

type PrimePopupPanel = {
  toggle: (event: Event) => void
  hide?: () => void
}

defineProps<{ items: readonly AdminRowActionItem[] }>()
const emit = defineEmits<{
  select: [key: string]
  hide: []
}>()

const popoverRef = ref<PrimePopupPanel | null>(null)
const popoverPt = {
  ...createAdminPopoverPt(),
  content: { style: { padding: '0' } },
}

function toggle(event: Event) {
  popoverRef.value?.toggle(event)
}

function hide() {
  popoverRef.value?.hide?.()
}

defineExpose({ toggle, hide })
</script>
