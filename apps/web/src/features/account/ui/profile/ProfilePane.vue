<template>
  <AccountPane :title="t('account.tabs.profile')">
    <section class="min-w-0" :aria-label="t('account.profile.overviewAria')">
      <div class="flex min-w-0 items-start gap-5 sm:items-center">
        <div
          class="relative size-20 shrink-0 overflow-visible rounded-pill bg-card shadow-raised sm:size-[88px]"
        >
          <button
            type="button"
            class="group relative flex h-full w-full cursor-pointer items-center justify-center overflow-hidden rounded-pill border-0 bg-transparent p-0 text-primary-foreground outline-none transition-shadow duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] focus-visible:shadow-focus-ring disabled:cursor-wait"
            :disabled="isAvatarUploading"
            :aria-busy="isAvatarUploading"
            :aria-label="t('account.profile.changeAvatar')"
            :title="t('account.profile.changeAvatar')"
            @click="openAvatarPicker"
          >
            <Avatar class="h-full w-full">
              <AvatarImage
                v-if="sizedAvatarUrl"
                :src="sizedAvatarUrl"
                :alt="userStore.user?.display_name"
                class="rounded-pill object-cover"
              />
              <AvatarFallback
                class="bg-primary text-admin-summary-value font-bold text-primary-foreground sm:text-[34px]"
              >
                {{ userInitial }}
              </AvatarFallback>
            </Avatar>
            <span
              class="pointer-events-none absolute inset-0 flex items-center justify-center rounded-pill bg-[color-mix(in_srgb,var(--foreground)_52%,transparent)] transition-opacity duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
              :class="
                isAvatarUploading
                  ? 'opacity-100'
                  : 'opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100'
              "
              aria-hidden="true"
            >
              <Camera class="size-6" :class="isAvatarUploading ? 'animate-pulse' : ''" />
            </span>
          </button>
          <input
            id="account-avatar-file"
            ref="avatarInputRef"
            class="hidden"
            type="file"
            name="avatar-file"
            accept="image/*"
            :disabled="isAvatarUploading"
            @change="onAvatarFileChange"
          />
        </div>

        <div class="min-w-0">
          <div v-if="isEditingName" class="flex min-w-0 items-center gap-2">
            <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The rename input carries its accessible name via aria-label -->
            <Input
              ref="renameInputRef"
              v-model="draftName"
              size="sm"
              class="h-9 max-w-xs text-sm"
              :disabled="isRenamingName"
              :aria-label="renameAriaLabel"
              @keydown="handleRenameKeydown"
            />
            <Button
              type="button"
              size="sm"
              class="h-9 shrink-0"
              :is-loading="isRenamingName"
              :is-disabled="isRenamingName"
              @click="commitRename"
            >
              {{ t('common.actions.save') }}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              class="h-9 shrink-0"
              :is-disabled="isRenamingName"
              @click="cancelRename"
            >
              {{ t('common.actions.cancel') }}
            </Button>
          </div>
          <div v-else class="flex min-w-0 items-center gap-2">
            <p
              class="m-0 min-w-0 flex-1 truncate text-[24px] leading-tight font-bold tracking-normal text-foreground sm:text-[30px]"
            >
              {{ currentDisplayName }}
            </p>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              class="size-7 shrink-0 text-muted-foreground hover:text-foreground"
              :title="renameAriaLabel"
              :aria-label="renameAriaLabel"
              @click="handleStartRename"
            >
              <EditPencil class="size-3.5" aria-hidden="true" />
            </Button>
          </div>
          <div class="mt-3 flex min-w-0 flex-wrap items-center gap-2">
            <StatusBadge v-if="isUserBanned" tone="danger" :dot="false">
              {{ t('account.profile.banned') }}
            </StatusBadge>
            <StatusBadge :tone="userRoleTone" :dot="false">
              {{ userRoleLabel }}
            </StatusBadge>
          </div>
        </div>
      </div>
    </section>

    <section class="min-w-0" :aria-label="t('account.profile.accountInfoAria')">
      <dl class="m-0 border-y border-border">
        <div
          v-for="field in profileFields"
          :key="field.label"
          class="grid min-h-14 grid-cols-1 items-center gap-1 border-b border-border py-3 last:border-b-0 sm:grid-cols-[148px_minmax(0,1fr)] sm:gap-4"
        >
          <dt class="text-admin-body font-semibold text-muted-foreground">
            {{ field.label }}
          </dt>
          <dd class="m-0 min-w-0">
            <span
              class="block truncate text-admin-title-sm font-semibold text-foreground"
              :class="field.mono ? 'font-mono text-admin-content' : ''"
              :title="field.value"
            >
              {{ field.value }}
            </span>
          </dd>
        </div>
      </dl>
    </section>
  </AccountPane>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useTemplateRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { Camera, EditPencil } from '@iconoir/vue'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import StatusBadge, { type AppStatusTone } from '../../../../core/components/StatusBadge.vue'
import { useUserStore } from '../../../auth'
import { useAvatarUpload } from '../../session/profile/useAvatarUpload'
import { useDisplayNameRename } from '../../session/profile/useDisplayNameRename'
import { formatDottedIsoDate } from '../../../../core/utils/dateTime'
import { resolveSizedAvatarUrl } from '../../../../core/media/avatarUrl'
import AccountPane from '../shared/AccountPane.vue'

type ProfileField = {
  label: string
  value: string
  mono?: boolean
}

const { t } = useI18n()
const userStore = useUserStore()
const { isUploading: isAvatarUploading, handleFileChange: onAvatarFileChange } = useAvatarUpload()
const {
  isEditing: isEditingName,
  isRenaming: isRenamingName,
  draftName,
  startRename,
  cancelRename,
  commitRename,
} = useDisplayNameRename()
const avatarInputRef = ref<HTMLInputElement | null>(null)
const renameInputRef = useTemplateRef('renameInputRef')

const currentDisplayName = computed(() => userStore.user?.display_name ?? '')
const renameAriaLabel = computed(() =>
  t('account.profile.renameAriaLabel', { name: currentDisplayName.value }),
)

const sizedAvatarUrl = computed(() => resolveSizedAvatarUrl(userStore.user?.avatar, 88))
const isUserBanned = computed(() => userStore.user?.status === 0)
const userRoleLabel = computed(() =>
  userStore.user?.role === 'admin'
    ? t('account.profile.roleAdmin')
    : t('account.profile.roleCreator'),
)
const userRoleTone = computed<AppStatusTone>(() =>
  userStore.user?.role === 'admin' ? 'info' : 'neutral',
)
const userInitial = computed(() => {
  const displayName = userStore.user?.display_name?.trim()
  return displayName?.[0]?.toUpperCase() || 'U'
})
const profileFields = computed<ProfileField[]>(() => [
  {
    label: t('account.profile.storageNamespace'),
    value: userStore.user?.storage_namespace || '-',
    mono: true,
  },
  {
    label: t('account.profile.joinedAt'),
    value: formatDottedIsoDate(userStore.user?.created_at),
  },
  {
    label: t('account.profile.lastUpdated'),
    value: formatDottedIsoDate(userStore.user?.updated_at),
  },
])

function openAvatarPicker() {
  avatarInputRef.value?.click()
}

function handleRenameKeydown(event: KeyboardEvent) {
  if (event.isComposing || event.keyCode === 229) return
  if (event.key === 'Enter') {
    event.preventDefault()
    void commitRename()
  } else if (event.key === 'Escape' || event.key === 'Esc') {
    event.preventDefault()
    event.stopPropagation()
    cancelRename()
  }
}

function handleStartRename() {
  startRename()
  void nextTick(() => {
    const host = renameInputRef.value as unknown as { $el?: unknown } | null
    const input = (host?.$el ?? host) as HTMLElement | null
    if (input instanceof HTMLInputElement) {
      input.focus()
      input.select()
    }
  })
}
</script>
