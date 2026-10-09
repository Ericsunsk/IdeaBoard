<template>
  <Teleport :to="teleportTarget" :disabled="!teleportTarget">
    <div
      class="mx-auto flex h-14 w-full max-w-admin-shell items-center justify-between gap-3 px-4 sm:px-6 lg:px-8"
    >
      <AdminTabs
        :model-value="activePath"
        class="min-w-0"
        @update:model-value="emit('navigate', $event)"
      >
        <AdminTabsList class="h-9">
          <AdminTabsTrigger
            v-for="item in items"
            :key="item.path"
            :value="item.path"
            class="h-7 text-xs sm:text-sm"
          >
            <slot name="item" :item="item">
              <span>{{ item.label }}</span>
            </slot>
          </AdminTabsTrigger>
        </AdminTabsList>
      </AdminTabs>
      <slot name="trailing" />
    </div>
  </Teleport>

  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <RouterView />
    <slot />
  </div>
</template>

<script setup lang="ts">
import { RouterView } from 'vue-router'
import { useAdminSubHeaderTarget } from '../composables/useAdminSubHeaderTarget'
import { AdminTabs, AdminTabsList, AdminTabsTrigger } from './adminTabsAtoms'

export type AdminSubNavigationItem = {
  key: string
  path: string
  label: string
}

defineProps<{
  items: readonly AdminSubNavigationItem[]
  activePath: string
}>()

const emit = defineEmits<{
  navigate: [path: string]
}>()

const teleportTarget = useAdminSubHeaderTarget()
</script>
