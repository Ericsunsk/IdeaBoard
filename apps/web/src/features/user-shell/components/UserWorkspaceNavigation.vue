<template>
  <nav
    class="absolute top-0 left-1/2 flex h-[var(--ib-page-chrome-control-size,40px)] w-[220px] -translate-x-1/2 items-center rounded-full border border-board-card-border bg-board-control-bg p-1 max-[860px]:static max-[860px]:order-3 max-[860px]:mt-4 max-[860px]:w-full max-[860px]:translate-x-0"
    aria-label="Workspace navigation"
  >
    <RouterLink
      v-for="item in navigationItems"
      v-slot="{ isActive }"
      :key="item.path"
      :to="item.path"
      exact-active-class="bg-ib-surface-hover text-board-page-text"
      class="flex h-full flex-1 items-center justify-center gap-1.5 rounded-full border border-transparent px-3 text-[13px] font-semibold text-board-page-text-muted no-underline transition-[background-color,color] duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] hover:text-board-page-text focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)]"
    >
      <component
        :is="item.icon"
        :class="['size-4 shrink-0', isActive ? 'text-board-primary' : 'text-current']"
        aria-hidden="true"
      />
      <span>{{ item.label }}</span>
    </RouterLink>
  </nav>
</template>

<script setup lang="ts">
import { Compass, ViewGrid } from '@iconoir/vue'
import { RouterLink } from 'vue-router'
import { USER_BOARDS_PATH, USER_SEARCH_PATH } from '../../../shared/constants/routePaths'

const navigationItems = [
  { path: USER_SEARCH_PATH, label: 'Search', icon: Compass },
  { path: USER_BOARDS_PATH, label: 'Workspace', icon: ViewGrid },
] as const
</script>
