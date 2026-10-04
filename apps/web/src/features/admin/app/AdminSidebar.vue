<template>
  <div>
    <!-- Desktop spacer keeps content width stable; collapsed hover expands as overlay -->
    <div
      class="hidden shrink-0 transition-[width] duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)] lg:block"
      :class="isCollapsed ? 'w-14' : 'w-52'"
    />

    <aside
      ref="siderRef"
      aria-label="后台导航"
      class="fixed top-0 left-0 z-50 flex h-screen shrink-0 flex-col justify-between overflow-hidden border-r border-border bg-card transition-[width,transform,box-shadow] duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)] lg:z-30"
      :class="[
        effectiveCollapsed ? 'w-52 lg:w-14' : 'w-52',
        isCollapsed && isHovered ? 'lg:z-40' : '',
        drawerOpen ? 'translate-x-0 shadow-xl' : '-translate-x-full lg:translate-x-0',
      ]"
      @mouseleave="handleNavMouseLeave"
      @focusout="handleSiderFocusOut"
    >
      <div
        class="flex min-h-0 flex-1 flex-col"
        @mouseenter="handleNavMouseEnter"
        @mouseleave="handleNavMouseLeave"
        @focusin="handleNavMouseEnter"
        @focusout="handleSiderFocusOut"
      >
        <div class="flex h-14 shrink-0 items-center border-b border-border px-2">
          <router-link
            :to="ADMIN_DEFAULT_PATH"
            :aria-label="brandAriaLabel"
            :title="effectiveCollapsed ? projectName || 'IdeaBoard' : undefined"
            class="group flex h-10 w-full min-w-0 items-center overflow-hidden rounded-xl whitespace-nowrap transition-colors hover:bg-muted active:scale-[0.98]"
          >
            <span
              class="flex size-10 shrink-0 items-center justify-center transition-transform duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)] group-hover:scale-105"
            >
              <SiteBrandLogo
                :src="logoURL"
                :fallback-text="fallbackText"
                :class="brandMarkClasses.logo"
              />
            </span>
            <span
              class="shrink-0 truncate pl-1.5 text-lg font-bold tracking-tight text-foreground transition-[opacity,transform]"
              :class="[
                effectiveCollapsed
                  ? 'pointer-events-none opacity-0 -translate-x-3 duration-[var(--motion-exit-duration)] ease-[var(--motion-exit-easing)]'
                  : 'opacity-100 translate-x-0 delay-75 duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)]',
              ]"
            >
              {{ projectName || 'IdeaBoard' }}
            </span>
          </router-link>
        </div>

        <nav
          class="flex min-h-0 flex-1 flex-col gap-1.5 overflow-x-hidden overflow-y-auto p-2"
          aria-label="后台功能"
        >
          <div
            v-for="(group, groupIndex) in adminNavigationGroups"
            :key="group.title"
            class="flex flex-col gap-1"
          >
            <div class="flex flex-col">
              <Transition
                enter-active-class="transition-[grid-template-rows,opacity] duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)]"
                enter-from-class="grid-rows-[0fr] opacity-0"
                enter-to-class="grid-rows-[1fr] opacity-100"
                leave-active-class="transition-[grid-template-rows,opacity] duration-[var(--motion-exit-duration)] ease-[var(--motion-exit-easing)]"
                leave-from-class="grid-rows-[1fr] opacity-100"
                leave-to-class="grid-rows-[0fr] opacity-0"
              >
                <div v-if="groupIndex > 0 && effectiveCollapsed" class="grid">
                  <div class="min-h-0 overflow-hidden">
                    <div class="mx-2 my-1 h-px bg-border" aria-hidden="true" />
                  </div>
                </div>
              </Transition>

              <Transition
                enter-active-class="transition-[grid-template-rows,opacity,transform] duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)]"
                enter-from-class="grid-rows-[0fr] opacity-0 -translate-x-3 -translate-y-2 scale-95"
                enter-to-class="grid-rows-[1fr] opacity-100 translate-x-0 translate-y-0 scale-100"
                leave-active-class="transition-[grid-template-rows,opacity,transform] duration-[var(--motion-exit-duration)] ease-[var(--motion-exit-easing)]"
                leave-from-class="grid-rows-[1fr] opacity-100 translate-x-0 translate-y-0 scale-100"
                leave-to-class="grid-rows-[0fr] opacity-0 -translate-x-3 -translate-y-2 scale-95"
              >
                <div v-if="!effectiveCollapsed" class="grid origin-top-left">
                  <div class="min-h-0 overflow-hidden">
                    <div
                      class="px-3 pt-2 pb-1 text-xs font-semibold tracking-wider text-muted-foreground select-none whitespace-nowrap"
                    >
                      {{ group.title }}
                    </div>
                  </div>
                </div>
              </Transition>
            </div>

            <router-link
              v-for="item in itemsOfGroup(group)"
              :key="item.key"
              :to="item.path"
              :title="effectiveCollapsed ? `${group.title} · ${item.label}` : item.label"
              :aria-label="item.label"
              :aria-current="activeKey === item.key ? 'page' : undefined"
              class="group flex h-10 w-full items-center overflow-hidden rounded-xl text-sm font-medium whitespace-nowrap text-foreground transition-colors hover:bg-muted active:scale-[0.98]"
              @click="drawerOpen = false"
            >
              <span class="flex size-10 shrink-0 items-center justify-center">
                <component
                  :is="item.icon"
                  :class="[
                    navIconSizeClass,
                    'text-muted-foreground transition-transform group-hover:scale-110',
                    activeKey === item.key && 'text-primary scale-105 font-bold',
                  ]"
                />
              </span>
              <span
                class="shrink-0 pl-1 origin-top-left transition-[opacity,transform]"
                :class="[
                  effectiveCollapsed
                    ? 'pointer-events-none opacity-0 -translate-x-3 -translate-y-1.5 scale-95 duration-[var(--motion-exit-duration)] ease-[var(--motion-exit-easing)]'
                    : 'opacity-100 translate-x-0 translate-y-0 scale-100 delay-75 duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)]',
                ]"
              >
                {{ item.label }}
              </span>
            </router-link>
          </div>
        </nav>
      </div>

      <div
        class="hidden h-14 shrink-0 items-center border-t border-border px-2 lg:flex"
        @mouseenter="handleBottomMouseEnter"
        @focusin="handleBottomMouseEnter"
      >
        <button
          type="button"
          :aria-label="isCollapsed ? '展开并固定侧边栏' : '折叠侧边栏'"
          :title="isCollapsed ? '展开并固定侧边栏' : '折叠侧边栏'"
          class="group flex size-10 items-center justify-center rounded-xl text-muted-foreground transition hover:bg-muted hover:text-foreground active:scale-95 cursor-pointer"
          @click="toggleCollapsed"
        >
          <SidebarCollapseIcon
            class="size-[18px] transition-transform"
            :class="isCollapsed ? 'rotate-180' : 'rotate-0'"
          />
        </button>
      </div>
    </aside>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { SidebarCollapse as SidebarCollapseIcon } from '@iconoir/vue'
import {
  adminNavigationByKey,
  adminNavigationGroups,
  ADMIN_DEFAULT_PATH,
  type AdminNavigationGroup,
  type AdminNavigationItem,
} from './navigation'
import SiteBrandLogo from '../../../shared/components/SiteBrandLogo.vue'
import { brandMarkClasses } from '../../../shared/lib/brandMark'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import { i18n } from '../../../shared/i18n'

defineProps<{
  activeKey: string
}>()

const drawerOpen = defineModel<boolean>('drawerOpen', { required: true })
const isCollapsed = ref(false)

const navIconSizeClass = 'size-4'

const isHovered = ref(false)
let hoverLeaveTimer: ReturnType<typeof setTimeout> | null = null
let hoverEnterTimer: ReturnType<typeof setTimeout> | null = null

function clearHoverTimers() {
  if (hoverLeaveTimer) {
    clearTimeout(hoverLeaveTimer)
    hoverLeaveTimer = null
  }
  if (hoverEnterTimer) {
    clearTimeout(hoverEnterTimer)
    hoverEnterTimer = null
  }
}

function handleNavMouseEnter() {
  clearHoverTimers()
  if (!isCollapsed.value) return
  hoverEnterTimer = setTimeout(() => {
    isHovered.value = true
    hoverEnterTimer = null
  }, 150)
}

function handleNavMouseLeave() {
  clearHoverTimers()
  if (!isHovered.value) return
  hoverLeaveTimer = setTimeout(() => {
    isHovered.value = false
    hoverLeaveTimer = null
  }, 120)
}

function handleBottomMouseEnter() {
  clearHoverTimers()
  isHovered.value = false
}

const effectiveCollapsed = computed(() => isCollapsed.value && !isHovered.value)

const { fallbackText, logoURL, projectName } = useSiteBranding({
  enabled: ref(true),
  fallback: i18n.global.t('account.branding.loadFailed'),
})
const brandAriaLabel = computed(() => `${projectName.value} Admin`)

function itemsOfGroup(group: AdminNavigationGroup): AdminNavigationItem[] {
  return group.keys
    .map((key) => adminNavigationByKey[key])
    .filter((item): item is AdminNavigationItem => Boolean(item))
}

function toggleCollapsed() {
  clearHoverTimers()
  isHovered.value = false
  isCollapsed.value = !isCollapsed.value
  try {
    localStorage.setItem('admin_sidebar_collapsed', String(isCollapsed.value))
  } catch {
    // ignore private-mode storage failures
  }
}

const siderRef = ref<HTMLElement | null>(null)

function handleSiderFocusOut(event: FocusEvent) {
  const next = event.relatedTarget
  if (next instanceof HTMLElement && siderRef.value?.contains(next)) return
  clearHoverTimers()
  isHovered.value = false
}

defineExpose({
  resetHoverState: () => {
    clearHoverTimers()
    isHovered.value = false
  },
})

onMounted(() => {
  try {
    const saved = localStorage.getItem('admin_sidebar_collapsed')
    if (saved !== null) isCollapsed.value = saved === 'true'
  } catch {
    // ignore private-mode storage failures
  }
})

onUnmounted(() => {
  clearHoverTimers()
})
</script>
