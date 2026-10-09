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
        :loading="initialLoading"
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
                :disabled="isFieldBusy(data.provider as SearchProviderId)"
                :loading="testingState[data.provider as SearchProviderId]"
                class="w-full pr-9 font-mono"
              />
            </div>
          </template>
        </AdminColumn>

        <!-- Status 列（健康三态：正常/异常/未验证，载体统一 AdminStatusPill） -->
        <AdminColumn field="status" :header="STATUS_COLUMN_HEADER" :style="{ width: '105px' }">
          <template #body="{ data }">
            <!-- 变色走过渡：Badge 本体自带 transition-colors，包一层把过渡带给圆点
                 span，test_state 翻转时不硬切。 -->
            <div class="[&_span]:transition-colors">
              <AdminStatusPill
                :label="testStateLabel(data.test_state)"
                :variant="testStateVariant(data.test_state)"
                size="small"
              />
            </div>
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
            </div>
          </template>
        </AdminColumn>

        <AdminColumn field="enabled" :header="M.enableColumn" :style="{ width: '80px' }">
          <template #body="{ data }">
            <AdminSwitch
              :model-value="data.enabled"
              :aria-label="`Enable ${SEARCH_PROVIDER_LABELS[data.provider as SearchProviderId]}`"
              :disabled="!canToggleEnable(data)"
              @update:model-value="toggleEnabled(data.provider as SearchProviderId, $event)"
            />
          </template>
        </AdminColumn>
      </AdminTable>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Search } from '@iconoir/vue'
import ActionButton from '../../../../shared/components/ActionButton.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import AdminStatusPill from '../../shared/components/AdminStatusPill.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import { useOptimisticToggle } from '../../shared/composables/useOptimisticToggle'
import { reconcileStableRows } from '../../shared/lib/reconcileStableRows'
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

// Key writes invalidate the old test result. Hold the reset (and any subsequent
// probe result) until props reflect it, so stale passed props cannot re-enable.
const keyTestStates = ref(new Map<SearchProviderId, SearchProviderTestState>())

// Toggle 独立 pending + 局部乐观 enabled 收口到共享 useOptimisticToggle：
// intent 同 tick 翻转开关（不等父级 refetch），pending 禁用同行操作但不点亮
// 保存按钮的 loading spinner（Button 内嵌 Spinner 会撑宽操作列，把 80px
// 启用列的开关顶偏）；成功后保留覆盖等回填 reconcile，失败回滚并走调用方
// feedback toast。开关本身只有 disabled（opacity），不改变布局。
const toggle = useOptimisticToggle<SearchProviderId>()

// 健康三态（方案 A）：Ready/Error/Unverified；启用只用开关，无启用胶囊。
const STATUS_COLUMN_HEADER = 'Status'
const TEST_STATE_LABEL: Record<SearchProviderTestState, string> = {
  passed: 'Ready',
  failed: 'Error',
  never: 'Unverified',
}
const TEST_STATE_VARIANT: Record<SearchProviderTestState, 'success' | 'danger' | 'neutral'> = {
  passed: 'success',
  failed: 'danger',
  never: 'neutral',
}

function testStateLabel(state: SearchProviderTestState): string {
  return TEST_STATE_LABEL[state]
}

function testStateVariant(state: SearchProviderTestState): 'success' | 'danger' | 'neutral' {
  return TEST_STATE_VARIANT[state]
}

// 初载才把 loading 透给 AdminTable：toggle 回填的后台 refetch（有行 + loading）
// 不再触发全行 opacity-70 弱示意，避免整表闪一下；空表初载骨架/loading 不变。
const initialLoading = computed(() => props.loading && props.configs.length === 0)

// 行引用缓存：key 是 data-key="provider"，内容一致即复用，杜绝回填整体替换
// 导致的内容重绘。
const rowCache = new Map<SearchProviderId, SearchProviderConfig>()

function isSameRow(cached: SearchProviderConfig, incoming: SearchProviderConfig): boolean {
  return (
    cached.provider === incoming.provider &&
    cached.enabled === incoming.enabled &&
    cached.api_key_configured === incoming.api_key_configured &&
    cached.test_state === incoming.test_state &&
    cached.failure_category === incoming.failure_category &&
    cached.status_message === incoming.status_message &&
    cached.tested_at === incoming.tested_at &&
    cached.endpoint_url === incoming.endpoint_url &&
    cached.model === incoming.model
  )
}

const tableRows = computed(() => {
  const sorted = [...props.configs].sort(
    (left, right) =>
      SEARCH_PROVIDERS.indexOf(left.provider) - SEARCH_PROVIDERS.indexOf(right.provider),
  )
  // 行引用稳定：先把乐观覆盖投影进 enabled，再经共享 helper 对账；内容
  // 一致即复用旧引用，只有真正变化的行才返回新对象。非目标行不更新，
  // data-key="provider" 保证行 DOM 复用不 remount。
  const projected = sorted.map((config) => ({
    ...config,
    api_key_configured: keyTestStates.value.has(config.provider) || config.api_key_configured,
    test_state: keyTestStates.value.get(config.provider) ?? config.test_state,
    enabled: toggle.effective(config.provider, config.enabled),
  }))
  return reconcileStableRows(rowCache, projected, (row) => row.provider, isSameRow)
})

// 服务端追上乐观值后清掉覆盖，避免长期遮挡外部变更；不等则保留。
watch(
  () => props.configs,
  (configs) => {
    toggle.reconcileAll(
      configs.map((config) => ({ key: config.provider, enabled: config.enabled })),
    )
    for (const config of configs) {
      const localTestState = keyTestStates.value.get(config.provider)
      if (
        config.api_key_configured &&
        (localTestState === config.test_state ||
          (localTestState === 'never' && config.test_state === 'failed'))
      ) {
        keyTestStates.value.delete(config.provider)
      }
    }
  },
  { deep: true },
)

function searchProviderLogo(provider: SearchProviderId): string {
  return ALL_ADMIN_BRAND_LOGOS.find((logo) => logo.key === provider)?.logoSrc ?? ''
}

function isRowBusy(provider: SearchProviderId): boolean {
  return testingState.value[provider] || savingState.value[provider] || toggle.isPending(provider)
}

// Toggle pending preserves editable inputs and button geometry; actions use
// isRowBusy to keep the same provider single-flight without a button spinner.
function isFieldBusy(provider: SearchProviderId): boolean {
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
  const enabled = toggle.effective(provider, config.enabled)
  if (enabled) return true
  if (apiKeyDrafts.value[provider].trim() !== '') return false
  return config.test_state === 'passed'
}

async function runTest(provider: SearchProviderId) {
  const config = tableRows.value.find((row) => row.provider === provider)
  if (!config || isTestDisabled(config)) return
  testingState.value[provider] = true
  try {
    // Routine probe (backend read-guard): no confirm dialog, no operator
    // reason; the audit records an empty reason.
    const result = await resource.value.test(provider, { operatorReason: '' })
    if (keyTestStates.value.has(provider)) keyTestStates.value.set(provider, result.test_state)
    emit('change')
    feedback.testPassed(SEARCH_PROVIDER_LABELS[provider])
  } catch (err) {
    emit('change')
    feedback.testFailed(err)
  } finally {
    testingState.value[provider] = false
  }
}

async function saveKey(provider: SearchProviderId) {
  const key = apiKeyDrafts.value[provider].trim()
  // UI disabled state is not a sufficient concurrency boundary: a queued click
  // can still arrive after a toggle started. Keep all writes for one provider
  // single-flight at the handler boundary as well.
  if (!key || isRowBusy(provider)) return

  const providerLabel = SEARCH_PROVIDER_LABELS[provider]
  // Routine config write (backend ReasonOptional): save directly without
  // the confirm dialog; the audit records an empty reason.
  savingState.value[provider] = true
  try {
    const wasEnabled = tableRows.value.find((c) => c.provider === provider)?.enabled ?? false
    await resource.value.update(provider, { api_key: key, enabled: false }, { operatorReason: '' })
    apiKeyDrafts.value[provider] = ''
    // Key updates acknowledge disabled + unverified even before parent refetch.
    toggle.optimistic.value.set(provider, false)
    keyTestStates.value.set(provider, 'never')
    emit('change')
    feedback.keySaved(providerLabel, wasEnabled)
  } catch (err) {
    feedback.saveFailed(err)
  } finally {
    savingState.value[provider] = false
  }
}

async function toggleEnabled(provider: SearchProviderId, enabled: boolean) {
  // Keep the same provider's key save and enable toggle mutually exclusive even
  // when a stale event bypasses the rendered disabled state.
  const config = tableRows.value.find((row) => row.provider === provider)
  if (!config || !canToggleEnable(config)) return
  const previousOverride = toggle.optimistic.value.get(provider)
  const providerLabel = SEARCH_PROVIDER_LABELS[provider]
  // Routine config write (backend ReasonOptional): toggle directly without
  // the confirm dialog; the audit records an empty reason. 乐观翻转与
  // pending/回滚收口到共享 composable，失败 toast 由调用方 feedback 传入。
  await toggle.toggle(
    provider,
    enabled,
    (key, next) => resource.value.update(key, { enabled: next }, { operatorReason: '' }),
    {
      onSuccess: (_, next) => {
        emit('change')
        feedback.toggled(providerLabel, next)
      },
      onFailure: (_, next, err) => {
        if (previousOverride !== undefined) toggle.optimistic.value.set(provider, previousOverride)
        feedback.toggleFailed(err, next)
      },
    },
  )
}
</script>
