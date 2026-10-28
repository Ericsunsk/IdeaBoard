<template>
  <div
    class="inline-flex shrink-0 items-center gap-2 leading-none"
    :class="props.wrapClass"
    @pointerdown.stop
    @mousedown.stop
    @click.stop
  >
    <template v-if="isGuest">
      <button
        type="button"
        class="page-chrome-surface control-state [--control-active-transform:scale(0.97)] inline-flex h-9 items-center justify-center rounded-pill px-4 text-xs font-semibold text-foreground transition-[border-color,background-color,transform,color] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-[var(--color-page-chrome-border-hover)] hover:bg-[var(--color-page-chrome-bg-hover)] active:scale-[0.97]"
        :aria-label="t('auth.signIn')"
        @click="loginItem?.command?.()"
      >
        {{ t('auth.signIn') }}
      </button>
      <button
        type="button"
        class="control-state [--control-hover-transform:scale(1.02)] [--control-active-transform:scale(0.97)] inline-flex h-9 items-center justify-center rounded-pill border-0 bg-gradient-to-r from-[var(--color-auth-primary-from)] to-[var(--color-auth-primary-to)] px-4.5 text-xs font-semibold text-[var(--color-auth-primary-text)] shadow-auth-primary transition-[box-shadow,transform,opacity] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:shadow-auth-primary-hover active:scale-[0.97]"
        :aria-label="t('auth.signUpFree')"
        @click="registerItem?.command?.()"
      >
        {{ t('auth.signUpFree') }}
      </button>
    </template>
    <template v-else>
      <UserCreditBalanceButton v-if="showCredits && balance" :balance="balance" />
      <slot name="actions" />
      <DropdownMenu
        v-model:open="open"
        dir="rtl"
        :modal="false"
        @update:open="handleMenuOpenUpdate"
      >
        <DropdownMenuTrigger ref="triggerRef" as-child>
          <button
            type="button"
            :title="t('auth.menu.title')"
            :aria-label="t('auth.menu.title')"
            :class="props.avatarButtonClass"
          >
            <img
              v-if="props.avatarUrl"
              :src="props.avatarUrl"
              :class="props.avatarImageClass"
              :alt="t('auth.menu.avatarAlt')"
            />
            <span v-else :class="props.avatarPlaceholderClass">
              <User :class="props.avatarIconClass" />
            </span>
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="start"
          :side-offset="appOverlayMenuSideOffset"
          :collision-padding="appOverlayMenuSideOffset"
          :class="menuContentClass"
          @pointer-down-outside="handlePointerDownOutside"
          @focus-outside="keepSupportQrFocus"
        >
          <!-- 极简用户信息：展示当前登录账号邮箱 -->
          <template v-if="props.userEmail">
            <DropdownMenuLabel class="px-2.5 py-1.5 font-normal">
              <span class="block truncate text-xs text-muted-foreground">
                {{ props.userEmail }}
              </span>
            </DropdownMenuLabel>
            <DropdownMenuSeparator class="my-1" />
          </template>

          <DropdownMenuSub v-if="isWorkspaceRowVisible" v-model:open="isWorkspaceSubOpen">
            <div class="flex items-center gap-1 px-2.5 pt-1.5 pb-0.5">
              <span class="min-w-0 flex-1 truncate text-xs font-medium text-muted-foreground">
                {{ t('workspace.switcher.rowTitle') }}
              </span>
              <button
                type="button"
                class="flex h-8 w-8 shrink-0 cursor-pointer items-center justify-center rounded-control text-muted-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted hover:text-foreground focus-visible:shadow-focus-ring"
                :aria-label="t('workspace.switcher.manage')"
                :title="t('workspace.switcher.manage')"
                @click.stop="openWorkspaceSettings"
              >
                <Settings class="size-4" aria-hidden="true" />
              </button>
            </div>
            <DropdownMenuSubTrigger :class="[appOverlayMenuItemClass, props.menuItemClass]">
              <span class="truncate" :title="currentWorkspaceLabel">{{
                currentWorkspaceLabel
              }}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent
              :side-offset="appOverlayMenuSideOffset"
              :class="submenuContentClass"
            >
              <div
                v-if="isWorkspaceLoading"
                class="flex items-center gap-2 px-2.5 py-2"
                aria-live="polite"
              >
                <Spinner class="size-4 shrink-0" aria-hidden="true" />
                <span class="truncate text-xs text-muted-foreground">
                  {{ t('common.actions.loading') }}
                </span>
              </div>
              <template v-else-if="workspaceLoadError">
                <DropdownMenuItem
                  :class="[appOverlayMenuItemClass, props.menuItemClass]"
                  @select="handleWorkspaceRetry"
                >
                  <span class="truncate">{{ t('common.actions.retry') }}</span>
                </DropdownMenuItem>
              </template>
              <p
                v-else-if="visibleWorkspaces.length === 0"
                class="m-0 px-2.5 py-2 text-xs text-muted-foreground"
                aria-live="polite"
              >
                {{ t('workspace.switcher.empty') }}
              </p>
              <template v-else>
                <DropdownMenuItem
                  v-for="item in visibleWorkspaces"
                  :key="item.publicId"
                  :class="[
                    appOverlayMenuItemClass,
                    props.menuItemClass,
                    item.publicId === props.workspace?.selectedId &&
                      'bg-muted text-[var(--app-overlay-menu-item-hover-color,var(--foreground))] font-medium',
                  ]"
                  :data-control-state="
                    item.publicId === props.workspace?.selectedId ? 'active' : 'idle'
                  "
                  :aria-current="item.publicId === props.workspace?.selectedId ? 'true' : undefined"
                  @select="handleWorkspaceSelect(item.publicId)"
                >
                  <span class="truncate flex-1" :title="workspaceOptionLabel(item)">{{
                    workspaceOptionLabel(item)
                  }}</span>
                  <Check
                    v-if="item.publicId === props.workspace?.selectedId"
                    class="size-3.5 shrink-0 ml-auto"
                    aria-hidden="true"
                  />
                </DropdownMenuItem>
              </template>
            </DropdownMenuSubContent>
          </DropdownMenuSub>

          <DropdownMenuSeparator class="my-1" />

          <DropdownMenuItem
            v-for="item in nonDangerItems"
            :key="item.key ?? item.label"
            :data-control-tone="item.tone ?? 'default'"
            :class="[appOverlayMenuItemClass, props.menuItemClass]"
            @select="item.command?.()"
          >
            <span class="flex min-w-0 flex-1 items-center justify-between gap-4">
              <span class="truncate">{{ item.label }}</span>
            </span>
          </DropdownMenuItem>

          <DropdownMenuSub>
            <DropdownMenuSubTrigger :class="[appOverlayMenuItemClass, props.menuItemClass]">
              <span class="truncate">{{ t('auth.menu.appearance') }}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent
              :side-offset="appOverlayMenuSideOffset"
              :class="submenuContentClass"
            >
              <DropdownMenuItem
                v-for="item in themeItems"
                :key="item.value"
                :class="[
                  appOverlayMenuItemClass,
                  props.menuItemClass,
                  item.isActive &&
                    'bg-muted text-[var(--app-overlay-menu-item-hover-color,var(--foreground))] font-medium',
                ]"
                :data-control-state="item.isActive ? 'active' : 'idle'"
                :aria-current="item.isActive ? 'true' : undefined"
                @select="setTheme(item.value)"
              >
                <component :is="item.icon" class="size-4 shrink-0 mr-2" />
                <span class="truncate flex-1">{{ item.label }}</span>
                <Check v-if="item.isActive" class="size-3.5 shrink-0 ml-auto" />
              </DropdownMenuItem>
            </DropdownMenuSubContent>
          </DropdownMenuSub>

          <DropdownMenuSub>
            <DropdownMenuSubTrigger :class="[appOverlayMenuItemClass, props.menuItemClass]">
              <span class="truncate">{{ t('auth.menu.language') }}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent
              :side-offset="appOverlayMenuSideOffset"
              :class="submenuContentClass"
            >
              <DropdownMenuItem
                v-for="item in localeItems"
                :key="item.value"
                :class="[
                  appOverlayMenuItemClass,
                  props.menuItemClass,
                  item.isActive &&
                    'bg-muted text-[var(--app-overlay-menu-item-hover-color,var(--foreground))] font-medium',
                ]"
                :data-control-state="item.isActive ? 'active' : 'idle'"
                :aria-current="item.isActive ? 'true' : undefined"
                @select="handleLocaleSelect(item.value)"
              >
                <span class="truncate flex-1">{{ item.label }}</span>
                <Check v-if="item.isActive" class="size-3.5 shrink-0 ml-auto" />
              </DropdownMenuItem>
            </DropdownMenuSubContent>
          </DropdownMenuSub>

          <DropdownMenuSub
            v-if="supportChannels.length"
            @update:open="!$event && (activeSupportChannelId = null)"
          >
            <DropdownMenuSubTrigger :class="[appOverlayMenuItemClass, props.menuItemClass]">
              <span class="truncate">{{ t('auth.menu.support') }}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent
              :side-offset="appOverlayMenuSideOffset"
              :collision-padding="appOverlayMenuSideOffset"
              :class="submenuContentClass"
              @focus-outside="keepSupportQrFocus"
            >
              <SupportChannelQr
                v-for="channel in supportChannels"
                :key="channel.id"
                v-model:active-channel-id="activeSupportChannelId"
                :channel="channel"
              />
            </DropdownMenuSubContent>
          </DropdownMenuSub>

          <DropdownMenuSeparator class="my-1" />

          <DropdownMenuItem
            v-for="item in dangerItems"
            :key="item.key ?? item.label"
            data-control-tone="danger"
            :class="[
              appOverlayMenuItemClass,
              props.menuItemClass,
              appOverlayMenuDangerItemClass,
              props.menuDangerItemClass,
            ]"
            @select="item.command?.()"
          >
            <span class="flex min-w-0 flex-1 items-center justify-between gap-4">
              <span class="truncate">{{ item.label }}</span>
            </span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <WorkspaceSettingsDialog
        v-if="isWorkspaceRowVisible"
        :is-open="isWorkspaceSettingsOpen"
        :items="visibleWorkspaces"
        :selected-id="visibleSelectedId"
        :is-loading="isWorkspaceLoading"
        :load-error="workspaceLoadError"
        :name-max-length="workspaceNameMaxLength"
        :current-user-id="props.workspace?.currentUserId ?? null"
        :on-retry="handleWorkspaceDialogRetry"
        :on-create="handleWorkspaceCreate"
        :on-load-organizations="props.workspace?.onLoadOrganizations"
        :on-create-organization="props.workspace?.onCreateOrganization"
        :organization-panel="props.workspace?.organizationPanel ?? null"
        :on-rename="handleWorkspaceRename"
        :on-delete="handleWorkspaceDelete"
        @update:is-open="isWorkspaceSettingsOpen = $event"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useTemplateRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, HalfMoon, Laptop, Settings, SunLight, User } from '@iconoir/vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Spinner } from '@/components/ui/spinner'
import {
  appOverlayMenuDangerItemClass,
  appOverlayMenuItemClass,
  appOverlayMenuSideOffset,
} from '../../../../theme/appOverlayMenu'
import { pageChromeAccountMenuClasses } from '../../../../core/lib/pageChrome'
import { useMenuTriggerFocusRestore } from '../../../../core/composables/useMenuTriggerFocusRestore'
import { useTheme } from '../../../../theme/composables/useTheme'
import {
  getActiveUserLocale,
  setUserLocale,
  type SupportedLocale,
} from '../../../../core/i18n/userLocale'
import { LOCALE_MANIFEST } from '../../../../core/i18n/localeManifest'
import type { SupportChannel } from '../../../../core/composables/useSupportChannels'
import { useFailureToast } from '../../../../core/composables/useFailureToast'
import SupportChannelQr from './SupportChannelQr.vue'
import UserCreditBalanceButton from './UserCreditBalanceButton.vue'
import WorkspaceSettingsDialog from './WorkspaceSettingsDialog.vue'
import { isDefaultWorkspaceItem } from './workspaceMenu'
import type {
  UserAccountMenuItem,
  UserAccountMenuWorkspace,
  UserAccountMenuWorkspaceCreateInput,
  UserAccountMenuWorkspaceItem,
  UserIdeaBalanceControl,
} from './userAccountMenu.types'

const props = withDefaults(
  defineProps<{
    avatarUrl?: string | undefined
    userEmail?: string | undefined
    items: UserAccountMenuItem[]
    balance?: UserIdeaBalanceControl | undefined
    wrapClass?: string
    avatarButtonClass?: string
    avatarImageClass?: string
    avatarPlaceholderClass?: string
    avatarIconClass?: string
    menuItemClass?: string
    menuDangerItemClass?: string
    showCredits?: boolean
    supportChannels?: SupportChannel[]
    workspace?: UserAccountMenuWorkspace | undefined
  }>(),
  {
    avatarUrl: undefined,
    userEmail: undefined,
    balance: undefined,
    wrapClass: pageChromeAccountMenuClasses.wrap,
    avatarButtonClass: pageChromeAccountMenuClasses.avatarButton,
    avatarImageClass: pageChromeAccountMenuClasses.avatarImage,
    avatarPlaceholderClass: pageChromeAccountMenuClasses.avatarPlaceholder,
    avatarIconClass: pageChromeAccountMenuClasses.avatarIcon,
    menuItemClass: appOverlayMenuItemClass,
    menuDangerItemClass: appOverlayMenuDangerItemClass,
    showCredits: true,
    supportChannels: () => [],
    workspace: undefined,
  },
)

const open = defineModel<boolean>('open', { default: false })
const activeSupportChannelId = ref<string | null>(null)
const isWorkspaceSubOpen = ref(false)
const isWorkspaceSettingsOpen = ref(false)
const { locale: i18nLocale, t } = useI18n()
const { mode, setTheme } = useTheme()

const menuContentClass = 'ib-user-account-menu-panel w-auto min-w-36 max-w-56 [direction:ltr]'
const submenuContentClass =
  'ib-user-account-menu-panel w-32 min-w-32 max-w-[8.5rem] [direction:ltr]'

const themeItems = computed(() => [
  {
    value: 'light' as const,
    label: t('auth.menu.themeLight'),
    icon: SunLight,
    isActive: mode.value === 'light',
  },
  {
    value: 'dark' as const,
    label: t('auth.menu.themeDark'),
    icon: HalfMoon,
    isActive: mode.value === 'dark',
  },
  {
    value: 'system' as const,
    label: t('auth.menu.themeSystem'),
    icon: Laptop,
    isActive: mode.value === 'auto',
  },
])

const activeLocale = computed<SupportedLocale>(() => {
  const current = i18nLocale.value as SupportedLocale
  return current || getActiveUserLocale()
})

const localeItems = computed(() =>
  (Object.keys(LOCALE_MANIFEST) as SupportedLocale[]).map((locale) => ({
    value: locale,
    label: LOCALE_MANIFEST[locale].displayLabel,
    isActive: locale === activeLocale.value,
  })),
)

async function handleLocaleSelect(locale: SupportedLocale) {
  if (locale === activeLocale.value) return
  await setUserLocale(locale)
}

const nonDangerItems = computed(() => props.items.filter((item) => item.tone !== 'danger'))
const dangerItems = computed(() => props.items.filter((item) => item.tone === 'danger'))

const loginItem = computed(() => props.items.find((item) => item.key === 'login'))
const registerItem = computed(() => props.items.find((item) => item.key === 'register'))
const isGuest = computed(() => Boolean(loginItem.value && registerItem.value))

// Workspace switcher row: rendered inside the dropdown for signed-in menus
// that receive workspace state from their shell header (workspace + admin
// alike). Guests never see a dropdown, so the row stays hidden for them.
const WORKSPACE_NAME_MAX_LENGTH_FALLBACK = 120

const isWorkspaceRowVisible = computed(() => !isGuest.value && props.workspace != null)
const visibleWorkspaces = computed(() => props.workspace?.items ?? [])
const visibleSelectedId = computed(() => {
  const selectedId = props.workspace?.selectedId
  return selectedId && visibleWorkspaces.value.some((item) => item.publicId === selectedId)
    ? selectedId
    : null
})
const isWorkspaceLoading = computed(() => props.workspace?.status === 'loading')
const workspaceLoadError = computed(() =>
  props.workspace?.status === 'error'
    ? (props.workspace.errorMessage ?? t('workspace.switcher.loadFailed'))
    : null,
)
// Own the shared list failure feedback for both the dropdown and settings dialog.
useFailureToast(workspaceLoadError, (message) => String(message ?? ''))
const workspaceNameMaxLength = computed(
  () => props.workspace?.nameMaxLength ?? WORKSPACE_NAME_MAX_LENGTH_FALLBACK,
)
function workspaceOptionLabel(item: UserAccountMenuWorkspaceItem): string {
  return isDefaultWorkspaceItem(item) ? t('workspace.switcher.defaultBadge') : item.name
}

const currentWorkspaceLabel = computed(() => {
  const selected = visibleWorkspaces.value.find((item) => item.publicId === visibleSelectedId.value)
  if (!selected) return t('workspace.switcher.title')
  return workspaceOptionLabel(selected)
})

function handleWorkspaceSelect(workspaceId: string) {
  isWorkspaceSubOpen.value = false
  open.value = false
  props.workspace?.onSelect(workspaceId)
}

function handleWorkspaceRetry(event: Event) {
  // Stay in the menu so the reloaded list lands where the user is looking.
  event.preventDefault()
  props.workspace?.onRetry()
}

function openWorkspaceSettings() {
  // The submenu and the settings modal never share the screen: opening the
  // modal closes the submenu and the dropdown with it.
  isWorkspaceSubOpen.value = false
  open.value = false
  isWorkspaceSettingsOpen.value = true
}

function handleWorkspaceDialogRetry() {
  props.workspace?.onRetry()
}

function requireWorkspace(): UserAccountMenuWorkspace {
  const workspace = props.workspace
  if (!workspace) throw new Error('Workspace controls are unavailable')
  return workspace
}

async function handleWorkspaceCreate(input: UserAccountMenuWorkspaceCreateInput) {
  // Creation preserves the current route and workspace scope; the settings
  // dialog stays open and focuses its new row (with Members) locally.
  return requireWorkspace().onCreate(input)
}

async function handleWorkspaceRename(workspaceId: string, name: string) {
  return requireWorkspace().onRename(workspaceId, name)
}

async function handleWorkspaceDelete(workspaceId: string) {
  return requireWorkspace().onDelete(workspaceId)
}

function keepSupportQrFocus(event: Event) {
  if (event.target instanceof Element && event.target.closest('[data-support-qr-panel]'))
    event.preventDefault()
}

const triggerRef = useTemplateRef('triggerRef')
const { handleMenuOpenUpdate, handlePointerDownOutside } = useMenuTriggerFocusRestore(triggerRef)

defineExpose({
  closeMenu: () => {
    open.value = false
  },
})
</script>
