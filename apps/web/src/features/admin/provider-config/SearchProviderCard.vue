<template>
  <Card :class="[adminMasonryCardClass, 'min-w-0']" :pt="adminCardPt">
    <template #title>
      <div class="flex min-w-0 items-start justify-between gap-3">
        <!-- 头部左侧：搜索徽章 + 标题 + 副标题 -->
        <div class="flex min-w-0 items-center gap-3">
          <div
            class="flex size-10 shrink-0 items-center justify-center rounded-xl bg-admin-accent/10 border border-admin-accent/20 text-admin-accent font-bold text-sm select-none"
          >
            <Search class="size-5 text-admin-accent" />
          </div>
          <div class="min-w-0">
            <h2 class="m-0 truncate text-admin-title-sm font-bold text-admin-title-color">
              Search & Vision
            </h2>
            <div class="truncate text-admin-caption text-admin-subtitle-color">
              素材图库与视觉模型服务
            </div>
          </div>
        </div>

        <!-- 头部右侧：启用 / 未启用状态徽章 (统一尺寸与规范) -->
        <div class="flex shrink-0 items-center gap-1.5">
          <AdminStatusPill
            :label="hasEnabledProvider ? '启用' : '未启用'"
            :variant="hasEnabledProvider ? 'success' : 'neutral'"
          />
        </div>
      </div>
    </template>

    <template #content>
      <!-- 极简 Logo 横向排列，角标指示状态，Hover Tooltip 显示详情 -->
      <div class="flex items-center gap-3 pt-0.5">
        <button
          v-for="row in rows"
          :key="row.provider"
          v-tooltip.top="providerLabel(row.provider)"
          type="button"
          class="group relative flex size-13 items-center justify-center rounded-xl border border-admin-border-soft bg-white p-2.5 shadow-xs cursor-pointer transition-colors hover:border-admin-accent hover:shadow-sm focus-visible:outline-none focus-visible:border-admin-accent"
          :aria-label="providerLabel(row.provider)"
          @click="openConfigDialog(row)"
        >
          <img
            v-if="getProviderLogoSrc(row)"
            :src="getProviderLogoSrc(row)"
            :alt="providerLabel(row.provider)"
            class="size-full object-contain transition-transform group-hover:scale-105"
          />
          <span v-else class="text-sm font-bold text-admin-accent uppercase select-none">
            {{ providerLabel(row.provider).slice(0, 2) }}
          </span>

          <!-- 右上角状态角标圆点 -->
          <span class="absolute -top-1 -right-1 flex size-2.5">
            <span
              v-if="row.enabled"
              class="absolute inline-flex size-full animate-ping rounded-full bg-admin-success opacity-75"
            />
            <span
              class="relative inline-flex size-2.5 rounded-full ring-2 ring-white"
              :class="
                row.enabled
                  ? 'bg-admin-success'
                  : row.test_state === 'failed'
                    ? 'bg-admin-danger'
                    : 'bg-admin-text-3'
              "
            />
          </span>
        </button>
      </div>
    </template>
  </Card>

  <!-- 搜索与视觉服务独立配置弹窗 -->
  <AdminDialogShell
    :visible="showConfigDialog"
    :header="
      editingRow
        ? `${providerLabel(editingRow.provider)} · ${providerCategoryLabel(editingRow.provider)}配置`
        : '服务配置'
    "
    :busy="savingProvider === editingRow?.provider"
    @update:visible="setConfigDialogVisible"
    @reset="resetEditingProvider"
  >
    <template #header>
      <div v-if="editingRow" class="flex min-w-0 items-center gap-3">
        <button
          type="button"
          :class="[adminLogoButtonClass, 'size-11 p-2 shrink-0 rounded-xl shadow-xs']"
          :data-control-state="activeLogoTarget === editingRow.provider ? 'selected' : 'idle'"
          :aria-expanded="activeLogoTarget === editingRow.provider ? 'true' : 'false'"
          title="点击切换品牌 Logo"
          aria-label="切换品牌 Logo"
          @click="openLogoPicker($event, editingRow.provider)"
        >
          <img
            v-if="getProviderLogoSrc(editingRow)"
            :src="getProviderLogoSrc(editingRow)"
            :alt="providerLabel(editingRow.provider)"
            class="size-full object-contain"
          />
          <span v-else class="text-xs font-bold text-admin-accent uppercase select-none">
            {{ providerLabel(editingRow.provider).slice(0, 2) }}
          </span>
        </button>
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <h2 class="m-0 truncate text-admin-title-sm font-bold text-admin-title-color">
              {{ providerLabel(editingRow.provider) }}
            </h2>
            <span
              class="rounded-full bg-admin-surface-muted px-2 py-0.5 text-[11px] font-semibold text-admin-text-2 border border-admin-border-soft"
            >
              {{ providerCategoryLabel(editingRow.provider) }}
            </span>
          </div>
          <p class="m-0 truncate text-admin-caption font-mono text-admin-subtitle-color">
            {{ providerHost(editingRow.provider) }}
          </p>
        </div>
      </div>
    </template>

    <div v-if="editingRow" class="flex flex-col gap-3.5 pt-1">
      <!-- 服务启用开关面板 -->
      <div
        class="flex items-center justify-between rounded-xl border border-admin-border-soft bg-admin-surface-muted/50 px-4 py-3"
      >
        <span class="text-admin-body font-semibold text-admin-title-color">启用该服务</span>
        <div class="flex items-center gap-2.5">
          <span
            class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-semibold"
            :class="
              formEnabled
                ? 'bg-admin-success-soft text-admin-success'
                : 'bg-admin-surface-muted text-admin-text-3 border border-admin-border-soft'
            "
          >
            <span
              class="size-1.5 rounded-full"
              :class="formEnabled ? 'bg-admin-success animate-pulse' : 'bg-admin-text-3'"
            />
            {{ formEnabled ? '已启用' : '已停用' }}
          </span>
          <ToggleSwitch
            v-model="formEnabled"
            size="small"
            :disabled="!canEnableEditing"
            :aria-label="`${providerLabel(editingRow.provider)} 启用开关`"
          />
        </div>
      </div>

      <!-- Vision 专属配置区域：Endpoint URL & Model -->
      <template v-if="editingRow.provider === 'vision'">
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <AdminFormField label="Endpoint URL" for="admin-search-endpoint-url">
            <InputText
              id="admin-search-endpoint-url"
              v-model="formEndpointUrl"
              class="w-full !text-xs font-mono"
              placeholder="例如: https://gateway.ai.cloudflare.com/v1/..."
            />
          </AdminFormField>

          <AdminFormField label="Vision Model" for="admin-search-model">
            <InputText
              id="admin-search-model"
              v-model="formModel"
              class="w-full !text-xs font-mono"
              placeholder="例如: @cf/meta/llama-3.2-11b-vision-instruct 或 gpt-4o"
            />
          </AdminFormField>
        </div>
      </template>

      <!-- 通用 API Key 字段 -->
      <AdminFormField label="API Key" for="admin-search-api-key">
        <InputText
          id="admin-search-api-key"
          v-model="formApiKey"
          type="password"
          autocomplete="new-password"
          class="w-full !text-xs font-mono"
          :placeholder="
            editingRow.api_key_configured ? 'API Key 已配置 (留空保持不变)' : '输入 API Key'
          "
        />
      </AdminFormField>

      <!-- 连通状态或错误提示条 -->
      <div
        v-if="editingRow.status_message"
        class="flex items-center gap-2.5 rounded-xl px-3.5 py-2.5 text-xs transition-colors"
        :class="
          editingRow.test_state === 'failed'
            ? 'border border-admin-danger/20 bg-admin-danger-soft text-admin-danger'
            : editingRow.test_state === 'passed'
              ? 'border border-admin-success/20 bg-admin-success-soft text-admin-success'
              : 'border border-admin-border-soft bg-admin-surface-muted text-admin-text-2'
        "
      >
        <CheckCircle
          v-if="editingRow.test_state === 'passed'"
          class="size-4 shrink-0 text-admin-success"
        />
        <WarningTriangle
          v-else-if="editingRow.test_state === 'failed'"
          class="size-4 shrink-0 text-admin-danger"
        />
        <span class="truncate font-medium leading-relaxed">
          {{ editingRow.status_message }}
        </span>
      </div>

      <p v-if="testActionHint" class="m-0 text-admin-caption text-admin-text-2" role="status">
        {{ testActionHint }}
      </p>
    </div>

    <template #footer>
      <Button
        label="测试连接"
        severity="secondary"
        outlined
        :class="adminActionButtonClass"
        :loading="testingProvider === editingRow?.provider"
        :disabled="!canTestEditing || savingProvider === editingRow?.provider"
        @click="editingRow && test(editingRow)"
      />
      <Button
        label="取消"
        severity="secondary"
        outlined
        :class="adminActionButtonClass"
        @click="setConfigDialogVisible(false)"
      />
      <Button
        label="保存配置"
        severity="contrast"
        :class="adminActionButtonClass"
        :loading="savingProvider === editingRow?.provider"
        :disabled="testingProvider === editingRow?.provider"
        @click="saveEditingProvider"
      />
    </template>
  </AdminDialogShell>

  <!-- Logo 选择弹窗 -->
  <AdminLogoPickerPopover
    ref="logoPickerPopoverRef"
    scope="search-provider"
    :target-id="activeLogoTarget || undefined"
    :target-name="
      activeLogoTarget
        ? providerLabel(activeLogoTarget as SearchProviderRecord['provider'])
        : undefined
    "
    :target-base-url="
      activeLogoTarget
        ? formEndpointUrl || rows.find((r) => r.provider === activeLogoTarget)?.endpoint_url
        : undefined
    "
    initial-category="search"
    @hide="activeLogoTarget = null"
  />
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputText from 'primevue/inputtext'
import ToggleSwitch from 'primevue/toggleswitch'
import Tooltip from 'primevue/tooltip'
import { CheckCircle, Search, WarningTriangle } from '@iconoir/vue'
import AdminDialogShell from '../shared/components/AdminDialogShell.vue'
import AdminFormField from '../shared/components/AdminFormField.vue'
import AdminStatusPill from '../shared/components/AdminStatusPill.vue'
import { useAdminToast } from '../shared/lib/adminToast'
import { adminCardPt } from '../shared/lib/adminCardPt'
import {
  adminActionButtonClass,
  adminLogoButtonClass,
  adminMasonryCardClass,
} from '../shared/lib/adminUiClasses'
import {
  fetchSearchProviders,
  saveSearchProvider,
  testSearchProvider,
  type SearchProviderRecord,
} from './searchProviderResource'
import { getSearchProviderLogo } from './providerLogos'
import AdminLogoPickerPopover from './AdminLogoPickerPopover.vue'

const vTooltip = Tooltip
const toast = useAdminToast()
const loading = ref(false)
const savingProvider = ref<SearchProviderRecord['provider'] | ''>('')
const testingProvider = ref<SearchProviderRecord['provider'] | ''>('')
const rows = ref<SearchProviderRecord[]>([])
const hasEnabledProvider = computed(() => rows.value.some((row) => row.enabled))

// 独立配置弹窗状态
const showConfigDialog = ref(false)
const editingRow = ref<SearchProviderRecord | null>(null)
const formEnabled = ref(false)
const formApiKey = ref('')
const formEndpointUrl = ref('')
const formModel = ref('')

const activeLogoTarget = ref<string | null>(null)
const logoPickerPopoverRef = ref<InstanceType<typeof AdminLogoPickerPopover> | null>(null)

function getProviderLogoSrc(row: SearchProviderRecord): string | undefined {
  return getSearchProviderLogo({
    provider: row.provider,
    name: providerLabel(row.provider),
    baseUrl: row.endpoint_url,
  })
}

function openConfigDialog(row: SearchProviderRecord) {
  editingRow.value = row
  showConfigDialog.value = true
}

function setConfigDialogVisible(visible: boolean) {
  showConfigDialog.value = visible
  if (!visible) resetEditingProvider()
}

function resetEditingProvider() {
  if (!editingRow.value) return
  formEnabled.value = editingRow.value.enabled
  formApiKey.value = ''
  formEndpointUrl.value = editingRow.value.endpoint_url ?? ''
  formModel.value = editingRow.value.model ?? ''
}

function openLogoPicker(event: Event, providerKey: string) {
  activeLogoTarget.value = providerKey
  logoPickerPopoverRef.value?.toggle(event)
}

async function load() {
  loading.value = true
  try {
    rows.value = await fetchSearchProviders()
    if (editingRow.value) {
      const updated = rows.value.find((r) => r.provider === editingRow.value?.provider)
      if (updated) {
        editingRow.value = updated
      }
    }
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '加载搜索与视觉服务配置失败')
  } finally {
    loading.value = false
  }
}

async function saveEditingProvider() {
  if (!editingRow.value) return
  const provider = editingRow.value.provider
  savingProvider.value = provider
  try {
    await saveSearchProvider(provider, {
      apiKey: formApiKey.value,
      endpointUrl: formEndpointUrl.value,
      model: formModel.value,
      enabled: formEnabled.value,
    })
    formApiKey.value = ''
    toast.success(`${providerLabel(provider)} 配置已保存`)
    await load()
    setConfigDialogVisible(false)
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '保存搜索与视觉服务配置失败')
  } finally {
    savingProvider.value = ''
  }
}

async function test(row: SearchProviderRecord) {
  testingProvider.value = row.provider
  try {
    await testSearchProvider(row.provider)
    toast.success(`${providerLabel(row.provider)} 连接测试通过`)
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '连接测试失败')
  } finally {
    await load()
    testingProvider.value = ''
  }
}

const hasUnsavedProviderConfiguration = computed(() => {
  if (!editingRow.value) return false
  if (formApiKey.value.trim()) return true
  if (editingRow.value.provider !== 'vision') return false
  return (
    formEndpointUrl.value.trim() !== (editingRow.value.endpoint_url ?? '') ||
    formModel.value.trim() !== (editingRow.value.model ?? '')
  )
})

const canTestEditing = computed(() => {
  if (!editingRow.value) return false
  return editingRow.value.api_key_configured && !hasUnsavedProviderConfiguration.value
})

const canEnableEditing = computed(() => {
  if (!editingRow.value) return false
  return (
    editingRow.value.test_state === 'passed' &&
    editingRow.value.api_key_configured &&
    (editingRow.value.provider !== 'vision' ||
      Boolean(editingRow.value.endpoint_url && editingRow.value.model)) &&
    !hasUnsavedProviderConfiguration.value
  )
})

const testActionHint = computed(() => {
  if (!editingRow.value) return ''
  if (hasUnsavedProviderConfiguration.value) return '配置已修改，请先保存配置后再测试连接。'
  if (!editingRow.value.api_key_configured) return '请先保存 API Key，再测试连接。'
  return ''
})

function providerLabel(provider: SearchProviderRecord['provider']) {
  if (provider === 'unsplash') return 'Unsplash'
  if (provider === 'pexels') return 'Pexels'
  return 'Vision Search'
}

function providerCategoryLabel(provider: SearchProviderRecord['provider']) {
  if (provider === 'unsplash' || provider === 'pexels') return '素材图库'
  return '视觉模型'
}

function providerHost(provider: SearchProviderRecord['provider']) {
  if (provider === 'unsplash') return 'api.unsplash.com'
  if (provider === 'pexels') return 'api.pexels.com'
  return 'OpenAI-compatible vision endpoint'
}

onMounted(load)
</script>
