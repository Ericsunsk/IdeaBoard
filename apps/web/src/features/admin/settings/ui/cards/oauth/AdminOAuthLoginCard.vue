<template>
  <div class="flex flex-col gap-3">
    <!-- 头部：板块名 + 副标题，配置按钮右上 -->
    <div class="flex flex-wrap items-center justify-between gap-3 pb-1">
      <div class="flex min-w-0 flex-col gap-0.5">
        <div class="flex items-center gap-2">
          <component
            :is="SETTINGS_SECTIONS.oauth.icon"
            class="size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
          <h3 class="text-admin-title-sm font-bold text-foreground">
            {{ SETTINGS_SECTIONS.oauth.title }}
          </h3>
        </div>
        <p class="text-admin-caption text-muted-foreground">
          {{ M.oauthLoginDescription }}
        </p>
      </div>
      <ActionButton :disabled="query.isPending.value" @click="visible = true">{{
        M.oauthConfigure
      }}</ActionButton>
    </div>

    <div class="flex flex-col gap-3 rounded-panel border border-border bg-card p-5">
      <div v-if="query.isPending.value" class="flex items-center gap-2" role="status">
        <AdminSpinner class="size-4" :aria-label="M.settingsLoading" />
        <span class="text-admin-body text-muted-foreground">{{ M.settingsLoading }}</span>
      </div>
      <template v-else>
        <div
          v-for="provider in providers"
          :key="provider.code"
          class="flex items-center justify-between gap-3"
        >
          <div class="flex min-w-0 items-center gap-2">
            <img
              :src="provider.logo"
              alt=""
              class="size-5 shrink-0 object-contain"
              aria-hidden="true"
            />
            <span class="truncate text-admin-body font-medium text-foreground">{{
              provider.label
            }}</span>
          </div>
          <StatusBadge
            :label="OAUTH_MESSAGES.oauthEnabledLabel(provider.enabled)"
            :tone="provider.enabled ? 'success' : 'neutral'"
          />
        </div>
      </template>
    </div>

    <OAuthProviderSettingsDialog v-model:visible="visible" :resource="resource" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import ActionButton from '@/core/components/ActionButton.vue'
import { AdminSpinner } from '../../../../shared/index'
import { StatusBadge } from '../../../../shared/index'
import OAuthProviderSettingsDialog from './OAuthProviderSettingsDialog.vue'
import {
  createOAuthProviderSettingsResource,
  type OAuthProviderSettingsResource,
} from '../../../session/oauth/resource'
import {
  getOAuthProviderLabel,
  type OAuthProviderCode,
} from '@/core/auth/oauthProviders'
import { resolveOAuthProviderLogo } from './logo'
import { OAUTH_MESSAGES } from './messages'
import { SETTINGS_MESSAGES as M } from '../../pages/messages'
import { SETTINGS_SECTIONS } from '../../pages/sections'
import { useRefetchOnClose } from '../../../../shared/index'
import { formatAdminDisplayError, useAdminToast } from '../../../../shared/index'

const props = defineProps<{ resource?: OAuthProviderSettingsResource | undefined }>()
const resource = computed(() => props.resource ?? createOAuthProviderSettingsResource())
const visible = ref(false)
const toast = useAdminToast()

const PROVIDER_CODES: readonly OAuthProviderCode[] = ['google', 'discord']

const query = useQuery({
  queryKey: ['admin', 'settings', 'oauth-login'],
  queryFn: async () => {
    const [google, discord] = await Promise.all([
      resource.value.get('google'),
      resource.value.get('discord'),
    ])
    return { google, discord }
  },
})

const providers = computed(() =>
  PROVIDER_CODES.map((code) => ({
    code,
    label: getOAuthProviderLabel(code),
    logo: resolveOAuthProviderLogo(code),
    enabled:
      (code === 'google' ? query.data.value?.google : query.data.value?.discord)?.enabled ?? false,
  })),
)

// 弹窗内保存后关闭时刷新摘要，保持板块状态与弹窗一致。
useRefetchOnClose(visible, () => query.refetch())

// 加载失败走 toast，Status 胶囊保留为真相源。
watch(
  () => query.error.value,
  (error) => {
    if (error) toast.error(formatAdminDisplayError(error, OAUTH_MESSAGES.oauthLoadFailed))
  },
)
</script>
