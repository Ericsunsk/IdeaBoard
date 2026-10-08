<template>
  <div class="flex w-44 shrink-0 flex-col gap-2.5">
    <div class="px-3 text-admin-caption font-semibold text-muted-foreground select-none">
      {{ title }}
    </div>
    <nav class="relative flex flex-col border-l border-border/70" aria-label="本页内容导航">
      <button
        v-for="item in items"
        :key="item.id"
        type="button"
        :aria-current="activeId === item.id ? 'true' : undefined"
        class="group relative -ml-[2px] flex min-h-8 items-center gap-2 px-3 py-1.5 text-left text-admin-caption transition-colors cursor-pointer"
        :class="[
          activeId === item.id
            ? 'border-l-2 border-primary font-medium text-foreground'
            : 'border-l-2 border-transparent text-muted-foreground hover:text-foreground',
        ]"
        @click="scrollTo(item.id)"
      >
        <component
          :is="item.icon"
          v-if="item.icon"
          class="size-3.5 shrink-0 transition-colors"
          :class="
            activeId === item.id
              ? 'text-primary'
              : 'text-muted-foreground group-hover:text-foreground'
          "
          aria-hidden="true"
        />
        <span class="truncate">{{ item.title }}</span>
      </button>
    </nav>
  </div>
</template>

<script setup lang="ts">
import type { Component } from 'vue'

export interface OnThisPageItem {
  id: string
  title: string
  icon?: Component
}

withDefaults(
  defineProps<{
    items: readonly OnThisPageItem[]
    activeId?: string | undefined
    title?: string
  }>(),
  {
    activeId: undefined,
    title: '本页内容',
  },
)

const emit = defineEmits<{
  select: [id: string]
}>()

function scrollTo(id: string) {
  emit('select', id)
  const element = document.getElementById(id)
  if (element) {
    element.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
}
</script>
