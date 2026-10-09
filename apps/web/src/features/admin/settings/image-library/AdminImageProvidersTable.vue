<template>
  <div class="flex flex-col gap-3">
    <!-- 头部：板块名 + 副标题 -->
    <div class="flex flex-wrap items-center justify-between gap-3 pb-1">
      <div class="flex min-w-0 flex-col gap-0.5">
        <div class="flex items-center gap-2">
          <component
            :is="SETTINGS_SECTIONS.image.icon"
            class="size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
          <h3 class="text-admin-title-sm font-bold text-foreground">
            {{ SETTINGS_SECTIONS.image.title }}
          </h3>
        </div>
        <p class="text-admin-caption text-muted-foreground">
          {{ M.imageLibrarySubtitle }}
        </p>
      </div>
    </div>

    <!-- 全局标准表格 -->
    <div :class="adminTableCardClass">
      <AdminTable
        :value="tableRows"
        :loading="loading"
        data-key="provider"
        scrollable
        :frame-class="adminTableFrameClass"
        :table-style="adminTableStyle"
        :class="adminTableClass"
        :table-aria-label="M.imageLibraryTitle"
      >
        <template #empty>
          <AdminEmptyState :title="M.emptyConfigs" />
        </template>

        <!-- Service 列 -->
        <AdminColumn
          field="provider"
          :header="M.providerColumn"
          :style="{ width: '160px', minWidth: '140px' }"
        >
          <template #body="{ data }">
            <div class="flex flex-col items-start gap-1.5">
              <div class="flex items-center gap-2">
                <span
                  v-if="searchProviderLogo(data.provider as SearchProviderId)"
                  class="flex size-6 shrink-0 items-center justify-center rounded-md bg-white p-1 ring-1 ring-border"
                >
                  <img
                    :src="searchProviderLogo(data.provider as SearchProviderId)"
                    :alt="SEARCH_PROVIDER_LABELS[data.provider as SearchProviderId]"
                    class="size-full object-contain"
                  />
                </span>
                <Search v-else class="size-4 text-muted-foreground shrink-0" />
                <span class="font-medium text-admin-body text-foreground">
                  {{ SEARCH_PROVIDER_LABELS[data.provider as SearchProviderId] }}
                </span>
              </div>
              <AdminStatusPill
                v-if="data.api_key_configured"
                :label="M.searchApiKeyConfigured"
                variant="success"
                size="small"
              />
            </div>
          </template>
        </AdminColumn>

        <!-- API Key 列 -->
        <AdminColumn field="apiKey" :header="M.apiKeyColumn" :style="{ minWidth: '240px' }">
          <template #body="{ data }">
            <div class="flex min-w-0 flex-col gap-1.5">
              <AdminInput
                :id="`admin-search-${data.provider}-api-key`"
                v-model="apiKeyDrafts[data.provider as SearchProviderId]"
                type="password"
                size="small"
                autocomplete="off"
                :aria-label="
                  M.searchApiKeyLabel(SEARCH_PROVIDER_LABELS[data.provider as SearchProviderId])
                "
                :placeholder="data.api_key_configured ? '••••••••••••••••' : M.searchApiKeyRequired"
                :disabled="isRowBusy(data.provider as SearchProviderId)"
                class="w-full font-mono"
              />
              <div
                v-if="data.test_state === 'failed' || data.status_message"
                class="rounded-lg border border-destructive/20 bg-destructive/5 p-2 text-admin-caption"
              >
                <div class="text-destructive font-medium">
                  <span v-if="data.failure_category">{{
                    M.searchFailureCategory(data.failure_category)
                  }}</span>
                  <span v-if="data.tested_at">
                    {{
                      M.searchTestedAt(
                        Boolean(data.failure_category),
                        formatAdminDateTime(data.tested_at),
                      )
                    }}
                  </span>
                </div>
                <div v-if="data.status_message" class="mt-0.5 text-foreground">
                  {{ data.status_message }}
                </div>
              </div>
              <p
                v-else-if="data.test_state === 'passed' && data.tested_at"
                class="text-admin-caption text-muted-foreground"
              >
                {{ M.searchLastTestPassed(formatAdminDateTime(data.tested_at)) }}
              </p>
            </div>
          </template>
        </AdminColumn>

        <!-- Status 列（全局规范：任务监控页 TaskStatusCell 式 Token 胶囊 + 圆点） -->
        <AdminColumn field="status" header="Status" :style="{ width: '105px' }">
          <template #body="{ data }">
            <span
              class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-admin-caption font-semibold leading-none"
              :class="testStatePillClass(data.test_state)"
            >
              <span class="size-1.5 rounded-full" :class="testStateDotClass(data.test_state)" />
              <span>{{ testStateLabel(data.test_state) }}</span>
            </span>
          </template>
        </AdminColumn>

        <!-- 启用状态列 -->
        <!-- 操作列 -->
        <AdminColumn :header="M.actionsColumn" :style="{ width: '180px', minWidth: '170px' }">
          <template #body="{ data }">
            <div class="flex min-w-0 flex-col items-start gap-1.5">
              <div class="flex items-center gap-2">
                <ActionButton
                  variant="outline"
                  :disabled="isTestDisabled(data)"
                  :loading="testingState[data.provider as SearchProviderId]"
                  @click="runTest(data.provider as SearchProviderId)"
                >
                  {{ M.searchTestConnection }}
                </ActionButton>
                <ActionButton
                  :disabled="isSaveDisabled(data)"
                  :loading="savingState[data.provider as SearchProviderId]"
                  @click="saveKey(data.provider as SearchProviderId)"
                >
                  {{ M.searchSaveKey }}
                </ActionButton>
              </div>
              <p v-if="sequenceHint(data)" class="text-admin-caption text-muted-foreground">
                {{ sequenceHint(data) }}
              </p>
            </div>
          </template>
        </AdminColumn>

        <AdminColumn field="enabled" :header="M.enableColumn" :style="{ width: '80px' }">
          <template #body="{ data }">
            <AdminSwitch
              :model-value="data.enabled"
              :aria-label="`启用 ${SEARCH_PROVIDER_LABELS[data.provider as SearchProviderId]}`"
              :disabled="!canToggleEnable(data)"
              @update:model-value="toggleEnabled(data.provider as SearchProviderId, $event)"
            />
          </template>
        </AdminColumn>
      </AdminTable>
    </div>

    <!-- 错误反馈 -->
    <AdminAlert v-if="globalError" severity="error">
      {{ globalError }}
    </AdminAlert>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Search } from '@iconoir/vue'
import { formatAdminDateTime } from '../../../../shared/utils/dateTime'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminStatusPill from '../../shared/components/AdminStatusPill.vue'
import ActionButton from '../../../../shared/components/ActionButton.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import {
  ADMIN_PROBE_OPERATOR_REASON,
  confirmSettingsSave,
} from '../../shared/lib/confirmSettingsSave'
import { formatAdminDisplayError } from '../../shared/lib/toast'
import {
  adminTableCardClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableStyle,
} from '../../shared/lib/uiClasses'
import { SETTINGS_MESSAGES as M } from '../pages/messages'
import { SETTINGS_SECTIONS } from '../shared/sections'
import { ALL_ADMIN_BRAND_LOGOS } from '../../shared/logos/brandLogos'
import { createAdminSearchProviderResource } from './searchProviderResource'
import type { AdminSearchProviderResource } from './searchProviderResource'
import { useAdminSearchProviderFeedback } from '../shared/useSettingsFeedback'
import {
  SEARCH_PROVIDERS,
  SEARCH_PROVIDER_LABELS,
  type SearchProviderConfig,
  type SearchProviderId,
  type SearchProviderTestState,
} from './searchProviderTypes'

const props = withDefaults(
  defineProps<{
    configs?: readonly SearchProviderConfig[] | undefined
    resource?: AdminSearchProviderResource | undefined
    loading?: boolean
  }>(),
  { configs: () => [], resource: undefined, loading: false },
)

const emit = defineEmits<{
  change: []
}>()

const defaultResource = createAdminSearchProviderResource()
const feedback = useAdminSearchProviderFeedback()
const confirm = useAdminConfirm()
const resource = computed(() => props.resource ?? defaultResource)

const apiKeyDrafts = ref<Record<SearchProviderId, string>>({
  unsplash: '',
  pexels: '',
})

const testingState = ref<Record<SearchProviderId, boolean>>({
  unsplash: false,
  pexels: false,
})

const savingState = ref<Record<SearchProviderId, boolean>>({
  unsplash: false,
  pexels: false,
})

const globalError = ref('')

const TEST_STATE_STYLE: Record<
  SearchProviderTestState,
  { label: string; pill: string; dot: string }
> = {
  passed: {
    label: '连接正常',
    pill: 'bg-success-soft text-success ring-1 ring-success/20',
    dot: 'bg-success',
  },
  failed: {
    label: '测试失败',
    pill: 'bg-destructive/15 text-destructive ring-1 ring-destructive/20',
    dot: 'bg-destructive',
  },
  never: {
    label: '未测试',
    pill: 'bg-muted text-muted-foreground ring-1 ring-border',
    dot: 'bg-muted-foreground',
  },
}

function testStateLabel(state: SearchProviderTestState): string {
  return TEST_STATE_STYLE[state].label
}

function testStatePillClass(state: SearchProviderTestState): string {
  return TEST_STATE_STYLE[state].pill
}

function testStateDotClass(state: SearchProviderTestState): string {
  return TEST_STATE_STYLE[state].dot
}

const tableRows = computed(() => {
  return [...props.configs].sort(
    (left, right) =>
      SEARCH_PROVIDERS.indexOf(left.provider) - SEARCH_PROVIDERS.indexOf(right.provider),
  )
})

function searchProviderLogo(provider: SearchProviderId): string {
  return ALL_ADMIN_BRAND_LOGOS.find((logo) => logo.key === provider)?.logoSrc ?? ''
}

function isRowBusy(provider: SearchProviderId): boolean {
  return testingState.value[provider] || savingState.value[provider]
}

function isTestDisabled(config: SearchProviderConfig): boolean {
  const provider = config.provider
  return (
    isRowBusy(provider) || apiKeyDrafts.value[provider].trim() !== '' || !config.api_key_configured
  )
}

function isSaveDisabled(config: SearchProviderConfig): boolean {
  const provider = config.provider
  return isRowBusy(provider) || apiKeyDrafts.value[provider].trim() === ''
}

function canToggleEnable(config: SearchProviderConfig): boolean {
  const provider = config.provider
  if (isRowBusy(provider)) return false
  if (config.enabled) return true
  if (apiKeyDrafts.value[provider].trim() !== '') return false
  return config.test_state === 'passed'
}

// 测试指引按行计算，草稿输入优先于已保存状态。
// 未配置且无草稿时不提示，由输入框占位与保存按钮引导。
function sequenceHint(config: SearchProviderConfig): string {
  const draft = apiKeyDrafts.value[config.provider]?.trim() ?? ''
  if (!config.api_key_configured && draft === '') {
    return ''
  }
  if (draft !== '') {
    return M.searchSequenceKeyChanged
  }
  if (config.test_state !== 'passed') {
    return M.searchSequenceTestFirst
  }
  return M.searchSequenceReady
}

async function runTest(provider: SearchProviderId) {
  testingState.value[provider] = true
  globalError.value = ''
  try {
    await resource.value.test(provider, { operatorReason: ADMIN_PROBE_OPERATOR_REASON })
    emit('change')
    feedback.testPassed(SEARCH_PROVIDER_LABELS[provider])
  } catch (err) {
    emit('change')
    globalError.value = formatAdminDisplayError(err, M.searchTestFailed)
  } finally {
    testingState.value[provider] = false
  }
}

async function saveKey(provider: SearchProviderId) {
  const key = apiKeyDrafts.value[provider].trim()
  if (!key) return

  const providerLabel = SEARCH_PROVIDER_LABELS[provider]
  const operatorReason = await confirmSettingsSave(confirm, {
    title: M.searchSaveKeyTitle(providerLabel),
    message: M.searchSaveKeyMessage,
    confirmLabel: M.searchSaveKey,
  })
  if (!operatorReason) return

  savingState.value[provider] = true
  globalError.value = ''
  try {
    const wasEnabled = props.configs.find((c) => c.provider === provider)?.enabled ?? false
    await resource.value.update(provider, { api_key: key, enabled: false }, { operatorReason })
    apiKeyDrafts.value[provider] = ''
    emit('change')
    feedback.keySaved(providerLabel, wasEnabled)
  } catch (err) {
    globalError.value = formatAdminDisplayError(err, M.searchSaveKeyFailed)
  } finally {
    savingState.value[provider] = false
  }
}

async function toggleEnabled(provider: SearchProviderId, enabled: boolean) {
  const providerLabel = SEARCH_PROVIDER_LABELS[provider]
  const operatorReason = await confirmSettingsSave(confirm, {
    title: M.searchToggleTitle(enabled, providerLabel),
    message: enabled ? M.searchToggleEnableMessage : M.searchToggleDisableMessage,
    confirmLabel: M.searchEnableState(enabled),
  })
  if (!operatorReason) return

  savingState.value[provider] = true
  globalError.value = ''
  try {
    await resource.value.update(provider, { enabled }, { operatorReason })
    emit('change')
    feedback.toggled(providerLabel, enabled)
  } catch (err) {
    globalError.value = formatAdminDisplayError(err, M.searchToggleFailed(enabled))
  } finally {
    savingState.value[provider] = false
  }
}

watch(
  () => props.configs,
  () => {
    globalError.value = ''
  },
)
</script>
