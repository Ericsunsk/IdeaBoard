<template>
  <AdminDialogShell
    :visible="visible"
    :header="MODELS_MESSAGES.addHeader"
    :description="MODELS_MESSAGES.addDescription"
    size="wide"
    @update:visible="emit('update:visible', $event)"
    @close="handleClose"
  >
    <div class="flex flex-col gap-4">
      <!-- 顶部操作栏：供应商过滤 + 关键词搜索 + 重新探测 -->
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-border pb-3.5">
        <div class="flex flex-1 flex-wrap items-center gap-2.5">
          <!-- 供应商筛选 -->
          <div class="w-40 sm:w-44">
            <AdminSelect
              v-model="selectedProvider"
              :options="providerOptions"
              size="small"
              :placeholder="MODELS_MESSAGES.addProviderAll"
            />
          </div>

          <!-- 关键词搜索（共享搜索框：图标 + 清空 + 统一输入框 chrome） -->
          <AdminToolbarSearch
            v-model="searchQuery"
            :input-label="MODELS_MESSAGES.addSearchLabel"
            :placeholder="MODELS_MESSAGES.addSearchPlaceholder"
            class="min-w-[200px] max-w-xs flex-1"
          />
        </div>

        <!-- 重新探测按钮 -->
        <button
          type="button"
          class="inline-flex h-8 items-center gap-1.5 rounded-lg border border-border bg-card px-2.5 text-xs font-medium text-muted-foreground shadow-xs transition-colors hover:bg-muted hover:text-foreground cursor-pointer disabled:opacity-50"
          :disabled="loading"
          @click="loadDetectedModels"
        >
          <Refresh class="size-3.5" :class="{ 'animate-spin': loading }" />
          <span>{{ MODELS_MESSAGES.addRedetect }}</span>
        </button>
      </div>

      <!-- 错误提示 -->
      <AdminAlert v-if="error" severity="error">
        <div class="flex items-center justify-between gap-2">
          <span>{{ error }}</span>
          <button
            type="button"
            class="text-xs font-semibold underline hover:no-underline cursor-pointer"
            @click="loadDetectedModels"
          >
            {{ MODELS_MESSAGES.retry }}
          </button>
        </div>
      </AdminAlert>

      <!-- 探测加载中：单行文案，避免与副标题重复造成“两个状态”观感 -->
      <div
        v-if="loading"
        class="flex flex-col items-center justify-center py-16 text-muted-foreground"
      >
        <Refresh class="size-6 animate-spin text-primary mb-2.5" />
        <p class="text-xs font-medium text-muted-foreground">{{ MODELS_MESSAGES.addDetecting }}</p>
      </div>

      <!-- 结果区与 loading 互斥：必须包在 template v-else 里 -->
      <template v-else>
        <!-- 空结果提示：失败态只保留顶部 error alert（含重试），此处不再叠加 -->
        <div
          v-if="!error && filteredModels.length === 0"
          class="flex flex-col items-center justify-center py-14 text-muted-foreground text-center"
        >
          <Cube class="size-8 text-muted-foreground/60 mb-2" />
          <p class="text-xs font-medium text-muted-foreground">
            {{ emptyDetectionTitle }}
          </p>
          <p class="text-admin-caption text-muted-foreground mt-1 max-w-sm">
            {{ emptyDetectionHint }}
          </p>
        </div>

        <!-- 紧凑 Checkbox 列表表格 -->
        <div
          v-else-if="!error"
          class="max-h-[380px] overflow-y-auto rounded-lg border border-border bg-card"
        >
          <table class="w-full text-left text-xs border-collapse">
            <thead
              class="sticky top-0 z-10 bg-muted/80 backdrop-blur-xs border-b border-border text-muted-foreground font-medium"
            >
              <tr>
                <th class="w-10 px-3 py-2.5 text-center">
                  <input
                    type="checkbox"
                    class="size-3.5 rounded border-input text-primary focus:ring-primary cursor-pointer"
                    :checked="isAllSelected"
                    :indeterminate="isIndeterminate"
                    :disabled="availableFilteredCount === 0"
                    :aria-label="MODELS_MESSAGES.addSelectAllAria"
                    @change="toggleSelectAll"
                  />
                </th>
                <th class="px-3 py-2.5">{{ MODELS_MESSAGES.addColumnInfo }}</th>
                <th class="px-3 py-2.5 w-32">{{ MODELS_MESSAGES.addColumnProvider }}</th>
                <th class="px-3 py-2.5 w-24 text-right">{{ MODELS_MESSAGES.addColumnStatus }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/60">
              <tr
                v-for="model in filteredModels"
                :key="model.id"
                class="transition-colors hover:bg-muted/40 cursor-pointer"
                :class="{
                  'bg-primary/[0.03]': selectedModelIds.has(model.id),
                  'opacity-60 bg-muted/20 cursor-not-allowed': isModelExisting(model.id),
                }"
                @click="toggleRowSelection(model.id)"
              >
                <!-- Checkbox -->
                <td class="w-10 px-3 py-2.5 text-center" @click.stop>
                  <input
                    type="checkbox"
                    class="size-3.5 rounded border-input text-primary focus:ring-primary cursor-pointer disabled:cursor-not-allowed"
                    :checked="selectedModelIds.has(model.id)"
                    :disabled="isModelExisting(model.id)"
                    :aria-label="MODELS_MESSAGES.addSelectAria(model.name || model.id)"
                    @change="toggleModelSelection(model.id)"
                  />
                </td>

                <!-- 模型信息 -->
                <td class="px-3 py-2.5">
                  <div class="flex items-center gap-2.5 min-w-0">
                    <div
                      class="flex size-6 shrink-0 items-center justify-center rounded-md overflow-hidden"
                      :class="
                        rowBrandLogo(model)
                          ? 'bg-muted/60 border border-border/60'
                          : 'bg-primary/10 text-primary'
                      "
                    >
                      <img
                        v-if="rowBrandLogo(model)"
                        :src="rowBrandLogo(model)"
                        :alt="model.provider"
                        class="size-3.5 object-contain"
                        loading="lazy"
                        @error="markBrandLogoFailed(model.id)"
                      />
                      <Cube v-else class="size-3.5" />
                    </div>
                    <div class="flex flex-col gap-0.5 min-w-0">
                      <span class="font-medium text-foreground truncate">
                        {{ model.name || model.id }}
                      </span>
                      <span class="font-mono text-admin-caption text-muted-foreground truncate">
                        {{ model.id }}
                      </span>
                    </div>
                  </div>
                </td>

                <!-- 供应商 -->
                <td class="px-3 py-2.5">
                  <span
                    class="inline-flex items-center rounded-md px-1.5 py-0.5 text-admin-caption font-medium"
                    :class="modelProviderBadgeClass(model.provider)"
                  >
                    {{ model.provider }}
                  </span>
                </td>

                <!-- 状态 -->
                <td class="px-3 py-2.5 text-right">
                  <span
                    v-if="isModelExisting(model.id)"
                    class="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-admin-micro font-medium text-muted-foreground"
                  >
                    {{ MODELS_MESSAGES.addExisting }}
                  </span>
                  <span
                    v-else-if="selectedModelIds.has(model.id)"
                    class="inline-flex items-center gap-1 rounded-full bg-primary/10 px-2 py-0.5 text-admin-micro font-medium text-primary"
                  >
                    <Check class="size-3" />
                    {{ MODELS_MESSAGES.addSelected }}
                  </span>
                  <span
                    v-else
                    class="inline-flex items-center rounded-full bg-muted/40 px-2 py-0.5 text-admin-micro font-medium text-muted-foreground"
                  >
                    {{ MODELS_MESSAGES.addNotAdded }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </div>

    <!-- 底部操作与导航 -->
    <template #footer>
      <div class="flex w-full items-center justify-between">
        <div class="text-xs text-muted-foreground">
          <span v-if="selectedCount > 0">{{ MODELS_MESSAGES.addPickedCount(selectedCount) }}</span>
        </div>

        <div class="flex items-center gap-2">
          <AdminButton variant="outline" :disabled="pending" @click="emit('create')">
            {{ MODELS_MESSAGES.addManual }}
          </AdminButton>

          <AdminButton variant="secondary" :disabled="pending" @click="handleClose">
            {{ MODELS_MESSAGES.cancel }}
          </AdminButton>

          <AdminButton
            variant="primary"
            :disabled="selectedCount === 0 || pending"
            :loading="pending"
            @click="handleImportSelected"
          >
            {{ MODELS_MESSAGES.addImport(selectedCount) }}
          </AdminButton>
        </div>
      </div>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Check, Cube, Refresh } from '@iconoir/vue'
import AdminAlert from '../../../shared/components/AdminAlert.vue'
import AdminButton from '../../../shared/components/AdminButton.vue'
import AdminDialogShell from '../../../shared/components/AdminDialogShell.vue'
import AdminSelect from '../../../shared/components/AdminSelect.vue'
import AdminToolbarSearch from '../../../shared/components/AdminToolbarSearch.vue'
import {
  createAdminGatewayResource,
  type DetectedGatewayModel,
} from '../../infrastructure/adminGatewayResource'
import { resolveModelBrandLogo } from '../../../shared/logos/brandLogos'
import type { AdminModelItem } from '../../domain/catalogData'
import { modelProviderBadgeClass } from '../../domain/catalogData'
import { buildModelItemFromDetected } from '../../domain/modelForm'
import { MODELS_MESSAGES } from '../pages/messages'

const props = withDefaults(
  defineProps<{
    visible: boolean
    existingModels?: AdminModelItem[]
    pending?: boolean
  }>(),
  {
    existingModels: () => [],
    pending: false,
  },
)

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'import', items: AdminModelItem[]): void
  // 手动新增：社区目录未覆盖的 Provider 走 EditDialog 空表单入库。
  (e: 'create'): void
}>()

const catalogResource = createAdminGatewayResource().catalog

const detectedModels = ref<DetectedGatewayModel[]>([])
const loading = ref(false)
const error = ref('')
// 探测元信息：空结果时解释原因（未配置 / 已配置但社区无可用），失败走顶部 error alert。
const detectedMeta = ref<{ configured: number } | null>(null)
const searchQuery = ref('')
const selectedProvider = ref('all')
const selectedModelIds = ref<Set<string>>(new Set())

// 已有模型集合
const existingIdSet = computed(() => {
  return new Set(props.existingModels.map((m) => m.id))
})

function isModelExisting(modelId: string): boolean {
  return existingIdSet.value.has(modelId)
}

// 品牌 logo 经后端透传 models.dev SVG：单行加载失败（404/断网）时回落 Cube
// 占位，避免破图；失败态按模型标识记忆，不影响其他行。
const failedBrandLogoIds = ref<Set<string>>(new Set())

function rowBrandLogo(model: DetectedGatewayModel): string | undefined {
  if (failedBrandLogoIds.value.has(model.id)) return undefined
  return resolveModelBrandLogo(model)
}

function markBrandLogoFailed(modelId: string): void {
  failedBrandLogoIds.value.add(modelId)
}

// 供应商下拉项
const providerOptions = computed(() => {
  const providers = Array.from(new Set(detectedModels.value.map((m) => m.provider))).filter(Boolean)
  return [
    { label: MODELS_MESSAGES.addProviderAll, value: 'all' },
    ...providers.map((p) => ({ label: p, value: p })),
  ]
})

// 过滤后的模型
const filteredModels = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return detectedModels.value.filter((model) => {
    if (selectedProvider.value !== 'all' && model.provider !== selectedProvider.value) {
      return false
    }
    if (query) {
      const matchId = model.id.toLowerCase().includes(query)
      const matchName = (model.name || '').toLowerCase().includes(query)
      if (!matchId && !matchName) return false
    }
    return true
  })
})

// 过滤后且不在现有库中的模型
const availableFilteredModels = computed(() => {
  return filteredModels.value.filter((m) => !isModelExisting(m.id))
})

const availableFilteredCount = computed(() => availableFilteredModels.value.length)

const selectedCount = computed(() => {
  return selectedModelIds.value.size
})

// 全选与半选状态
const isAllSelected = computed(() => {
  if (availableFilteredCount.value === 0) return false
  return availableFilteredModels.value.every((m) => selectedModelIds.value.has(m.id))
})

const isIndeterminate = computed(() => {
  if (availableFilteredCount.value === 0) return false
  const count = availableFilteredModels.value.filter((m) => selectedModelIds.value.has(m.id)).length
  return count > 0 && count < availableFilteredCount.value
})

function toggleSelectAll() {
  const next = new Set(selectedModelIds.value)
  if (isAllSelected.value) {
    for (const m of availableFilteredModels.value) {
      next.delete(m.id)
    }
  } else {
    for (const m of availableFilteredModels.value) {
      next.add(m.id)
    }
  }
  selectedModelIds.value = next
}

function toggleModelSelection(id: string) {
  if (isModelExisting(id)) return
  const next = new Set(selectedModelIds.value)
  if (next.has(id)) {
    next.delete(id)
  } else {
    next.add(id)
  }
  selectedModelIds.value = next
}

function toggleRowSelection(id: string) {
  toggleModelSelection(id)
}

// 自动探测逻辑
async function loadDetectedModels() {
  loading.value = true
  error.value = ''
  try {
    const detection = await catalogResource.detect()
    detectedModels.value = detection.models
    detectedMeta.value = {
      configured: detection.configured_providers,
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : MODELS_MESSAGES.addDetectFailed
  } finally {
    loading.value = false
  }
}

// 空结果文案：按后端元信息区分“未配置”与“扫过无可用”，不再统一显示空列表。
const emptyDetectionTitle = computed(() => {
  if (searchQuery.value.trim()) return MODELS_MESSAGES.emptyNoMatch
  const meta = detectedMeta.value
  if (meta && meta.configured === 0) return MODELS_MESSAGES.addEmptyUnconfigured
  if (meta) return MODELS_MESSAGES.addEmptyNoneAvailable(meta.configured)
  return MODELS_MESSAGES.addEmptyNone
})

const emptyDetectionHint = computed(() => {
  const meta = detectedMeta.value
  if (meta && meta.configured === 0 && !searchQuery.value.trim()) {
    return MODELS_MESSAGES.addHintEnableFirst
  }
  return MODELS_MESSAGES.addHintCheckProviders
})

// 监听弹窗打开自动触发探测
watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      searchQuery.value = ''
      selectedProvider.value = 'all'
      selectedModelIds.value = new Set()
      if (detectedModels.value.length === 0) {
        void loadDetectedModels()
      }
    }
  },
  { immediate: true },
)

function handleClose() {
  emit('update:visible', false)
}

// 批量导入
function handleImportSelected() {
  const toImport: AdminModelItem[] = []
  for (const model of detectedModels.value) {
    if (selectedModelIds.value.has(model.id) && !isModelExisting(model.id)) {
      toImport.push(buildModelItemFromDetected(model))
    }
  }
  if (toImport.length > 0) {
    emit('import', toImport)
  }
}
</script>
