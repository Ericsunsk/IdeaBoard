<template>
  <div
    class="inline-flex shrink-0 items-center gap-2 leading-none"
    :class="wrapClass"
    @pointerdown.stop
    @mousedown.stop
    @click.stop
  >
    <slot name="before-balance" />
    <template v-if="isGuest">
      <Button
        unstyled
        type="button"
        class="ib-page-chrome-surface ib-control-state [--ib-control-active-transform:scale(0.97)] inline-flex h-9 items-center justify-center rounded-full px-4 text-xs font-semibold text-ib-text transition-[border-color,background-color,transform,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-[var(--color-ib-page-chrome-border-hover)] hover:bg-[var(--color-ib-page-chrome-bg-hover)] active:scale-[0.97]"
        :aria-label="t('auth.signIn')"
        @click="loginItem?.command?.()"
      >
        {{ t('auth.signIn') }}
      </Button>
      <Button
        unstyled
        type="button"
        class="ib-control-state [--ib-control-hover-transform:scale(1.02)] [--ib-control-active-transform:scale(0.97)] inline-flex h-9 items-center justify-center rounded-full border-0 bg-gradient-to-r from-[var(--color-ib-auth-primary-from)] to-[var(--color-ib-auth-primary-to)] px-4.5 text-xs font-semibold text-[var(--color-ib-auth-primary-text)] shadow-[var(--shadow-ib-auth-primary)] transition-[box-shadow,transform,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:shadow-[var(--shadow-ib-auth-primary-hover)] active:scale-[0.97]"
        :aria-label="t('auth.signUpFree')"
        @click="registerItem?.command?.()"
      >
        {{ t('auth.signUpFree') }}
      </Button>
    </template>
    <template v-else>
      <UserCreditBalanceButton v-if="showCredits && balance" :balance="balance" />
      <slot name="actions" />
      <Button
        ref="avatarButtonRef"
        unstyled
        type="button"
        :title="t('auth.menu.title')"
        :aria-label="t('auth.menu.title')"
        aria-haspopup="menu"
        :aria-expanded="isMenuOpen"
        :class="avatarButtonClass"
        @click="toggleMenu"
      >
        <img
          v-if="avatarUrl"
          :src="avatarUrl"
          :class="avatarImageClass"
          :alt="t('auth.menu.avatarAlt')"
        />
        <div v-else :class="[avatarPlaceholderClass, 'bg-transparent']">
          <User :class="avatarIconClass" />
        </div>
      </Button>
      <Menu
        ref="menuRef"
        :model="primeMenuItems"
        popup
        :append-to="appendTo"
        :pt="resolvedMenuPt"
        @show="handleShow"
        @hide="handleHide"
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
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { User } from '@iconoir/vue'
import Button from 'primevue/button'
import Menu from 'primevue/menu'
import type { MenuPassThroughOptions } from 'primevue/menu'
import type { MenuItem } from 'primevue/menuitem'
import { usePrimePopupMenuOverlayLifecycle } from '../composables/usePrimePopupMenuOverlayLifecycle'
import {
  appOverlayMenuDangerItemClass,
  appOverlayMenuItemClass,
  userAccountMenuPt,
} from '../theme/appOverlayMenu'
import { pageChromeAccountMenuClasses } from '../lib/pageChrome'
import UserCreditBalanceButton from './UserCreditBalanceButton.vue'
import type { UserAccountMenuItem, UserIdeaBalanceControl } from './userAccountMenu.types'

type AvatarButtonRef = HTMLElement | { $el?: HTMLElement }

const props = withDefaults(
  defineProps<{
    avatarUrl?: string | undefined
    items: UserAccountMenuItem[]
    balance?: UserIdeaBalanceControl | undefined
    wrapClass?: string
    avatarButtonClass?: string
    avatarImageClass?: string
    avatarPlaceholderClass?: string
    avatarIconClass?: string
    appendTo?: string
    menuPt?: MenuPassThroughOptions | undefined
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

const { t } = useI18n()
const avatarButtonRef = ref<AvatarButtonRef | null>(null)
const {
  popupRef: menuRef,
  isOpen: isMenuOpen,
  toggle: toggleMenuOverlay,
  handleShow,
  handleHide,
  select,
} = usePrimePopupMenuOverlayLifecycle({})
void menuRef
const resolvedMenuPt = computed(() => props.menuPt ?? userAccountMenuPt)

const loginItem = computed(() => props.items.find((item) => item.key === 'login'))
const registerItem = computed(() => props.items.find((item) => item.key === 'register'))
const isGuest = computed(() => Boolean(loginItem.value && registerItem.value))

const primeMenuItems = computed<MenuItem[]>(() =>
  props.items.map((item) => ({
    ...item,
    command: () => {
      select(item.command)
    },
  })),
)

function resolveAvatarButtonElement(): HTMLElement | undefined {
  const button = avatarButtonRef.value
  if (!button) return undefined
  return button instanceof HTMLElement ? button : button.$el
}

function toggleMenu(event: Event) {
  toggleMenuOverlay(event, resolveAvatarButtonElement())
}
</script>
