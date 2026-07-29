<template>
  <div
    class="inline-flex shrink-0 items-center gap-2 leading-none"
    :class="wrapClass"
    @pointerdown.stop
    @mousedown.stop
    @click.stop
  >
    <UserCreditBalanceButton v-if="showCredits && balance" :balance="balance" />
    <Button
      ref="avatarButtonRef"
      unstyled
      type="button"
      title="账户菜单"
      aria-label="账户菜单"
      aria-haspopup="menu"
      :aria-expanded="isMenuOpen"
      :class="avatarButtonClass"
      @click="toggleMenu"
    >
      <img v-if="avatarUrl" :src="avatarUrl" :class="avatarImageClass" alt="用户头像" />
      <div v-else :class="avatarPlaceholderClass">
        <User :class="avatarIconClass" />
      </div>
    </Button>
    <Menu
      ref="menuRef"
      :model="primeMenuItems"
      popup
      :append-to="appendTo"
      :pt="resolvedMenuPt"
      @show="isMenuOpen = true"
      @hide="isMenuOpen = false"
    >
      <template #item="{ item, props: itemProps }">
        <a
          v-bind="itemProps.action"
          :class="[menuItemClass, item.tone === 'danger' ? menuDangerItemClass : '']"
          data-control-state="idle"
          :data-control-tone="item.tone"
        >
          <span class="flex min-w-0 flex-1 items-center justify-between gap-4">
            <span class="truncate">{{ item.label }}</span>
          </span>
        </a>
      </template>
    </Menu>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { User } from '@iconoir/vue'
import Button from 'primevue/button'
import Menu from 'primevue/menu'
import type { MenuPassThroughOptions } from 'primevue/menu'
import type { MenuItem } from 'primevue/menuitem'
import { appOverlayMenuDangerItemClass, appOverlayMenuItemClass } from '../theme/appOverlayMenu'
import { pageChromeAccountMenuClasses } from '../lib/pageChrome'
import { userAccountMenuPt } from '../theme/userAccountMenuSkin'
import UserCreditBalanceButton from './UserCreditBalanceButton.vue'
import type { UserAccountMenuItem, UserIdeaBalanceControl } from './userAccountMenu.types'

type PrimePopupMenuRef = {
  toggle: (event: Event, target?: HTMLElement) => void
  hide: () => void
}

type AvatarButtonRef = HTMLElement | { $el?: HTMLElement }

const props = withDefaults(
  defineProps<{
    avatarUrl?: string
    items: UserAccountMenuItem[]
    balance?: UserIdeaBalanceControl
    wrapClass?: string
    avatarButtonClass?: string
    avatarImageClass?: string
    avatarPlaceholderClass?: string
    avatarIconClass?: string
    appendTo?: string
    menuPt?: MenuPassThroughOptions
    menuItemClass?: string
    menuDangerItemClass?: string
    showCredits?: boolean
  }>(),
  {
    avatarUrl: undefined,
    balance: undefined,
    wrapClass: pageChromeAccountMenuClasses.wrap,
    avatarButtonClass: pageChromeAccountMenuClasses.avatarButton,
    avatarImageClass: pageChromeAccountMenuClasses.avatarImage,
    avatarPlaceholderClass: pageChromeAccountMenuClasses.avatarPlaceholder,
    avatarIconClass: pageChromeAccountMenuClasses.avatarIcon,
    appendTo: 'body',
    menuPt: undefined,
    menuItemClass: appOverlayMenuItemClass,
    menuDangerItemClass: appOverlayMenuDangerItemClass,
    showCredits: true,
  },
)

const avatarButtonRef = ref<AvatarButtonRef | null>(null)
const menuRef = ref<PrimePopupMenuRef | null>(null)
const isMenuOpen = ref(false)
const resolvedMenuPt = computed(() => props.menuPt ?? userAccountMenuPt)

const primeMenuItems = computed<MenuItem[]>(() =>
  props.items.map((item) => ({
    ...item,
    command: () => {
      menuRef.value?.hide()
      item.command()
    },
  })),
)

function resolveAvatarButtonElement(): HTMLElement | undefined {
  const button = avatarButtonRef.value
  if (!button) return undefined
  return button instanceof HTMLElement ? button : button.$el
}

function toggleMenu(event: Event) {
  menuRef.value?.toggle(event, resolveAvatarButtonElement())
}
</script>
