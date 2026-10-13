<template>
  <PopoverRoot :open="isOpen" @update:open="handleOpenUpdate">
    <PopoverAnchor v-if="anchorEl" :reference="anchorEl" />
    <PopoverPortal>
      <PopoverContent
        class="z-admin-dialog min-w-[176px] rounded-panel border border-border bg-card p-1.5 text-foreground outline-none overlay-popover-motion"
        :side-offset="4"
        align="end"
      >
        <div class="flex flex-col gap-1">
          <template v-for="item in items" :key="item.key">
            <div v-if="item.divided" class="my-1 h-px bg-border" aria-hidden="true" />
            <button
              type="button"
              :disabled="item.disabled"
              :class="[
                'flex w-full items-center justify-start rounded-control px-3 py-2 text-admin-meta font-admin-strong transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer disabled:pointer-events-none disabled:opacity-40',
                item.danger
                  ? 'text-destructive hover:bg-destructive/10'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground',
              ]"
              @click="handleSelect(item.key)"
            >
              {{ item.label }}
            </button>
          </template>
        </div>
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { PopoverAnchor, PopoverContent, PopoverPortal, PopoverRoot } from 'reka-ui'

export type AdminRowActionItem = {
  key: string
  label: string
  disabled?: boolean
  divided?: boolean
  danger?: boolean
}

defineProps<{ items: readonly AdminRowActionItem[] }>()
const emit = defineEmits<{
  select: [key: string]
  hide: []
}>()

const isOpen = ref(false)
const anchorEl = ref<HTMLElement | null>(null)

function toggle(event: Event) {
  const target = (event.currentTarget || event.target) as HTMLElement | null
  anchorEl.value = target
  isOpen.value = !isOpen.value
}

function hide() {
  isOpen.value = false
  emit('hide')
}

function handleOpenUpdate(value: boolean) {
  isOpen.value = value
  if (!value) {
    emit('hide')
  }
}

function handleSelect(key: string) {
  emit('select', key)
  hide()
}

defineExpose({ toggle, hide })
</script>
