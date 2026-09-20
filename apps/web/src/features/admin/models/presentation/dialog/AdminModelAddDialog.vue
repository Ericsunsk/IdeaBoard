<template>
  <AdminDialogShell
    :visible="visible"
    header="添加模型"
    description="勾选需要接入的模型后提交添加。"
    size="wide"
    @update:visible="emit('update:visible', $event)"
    @close="handleClose"
  >
    <div class="flex flex-col gap-4">
      <!-- 顶部操作栏：供应商过滤 + 关键词搜索 + 重新探测 -->
      <div
        class="flex flex-wrap items-center justify-between gap-3 border-b border-admin-border-soft pb-3.5"
      >
        <div class="flex flex-1 flex-wrap items-center gap-2.5">
          <!-- 供应商筛选 -->
          <div class="w-40 sm:w-44">
            <AdminSelect
              v-model="selectedProvider"
              :options="providerOptions"
              size="small"
              placeholder="全部供应商"
            />
          </div>

          <!-- 关键词搜索（共享搜索框：图标 + 清空 + 统一输入框 chrome） -->
          <AdminToolbarSearch
            v-model="searchQuery"
            input-label="搜索模型名称或标识"
            placeholder="搜索模型名称或标识..."
            class="min-w-[200px] max-w-xs flex-1"
          />
        </div>

        <!-- 重新探测按钮 -->
        <button
          type="button"
          class="inline-flex h-8 items-center gap-1.5 rounded-lg border border-admin-border-soft bg-white px-2.5 text-xs font-medium text-admin-text-2 shadow-xs transition-colors hover:bg-admin-surface-muted hover:text-admin-text-1 cursor-pointer disabled:opacity-50"
          :disabled="loading"
          @click="loadDetectedModels"
        >
          <Refresh class="size-3.5" :class="{ 'animate-spin': loading }" />
          <span>重新探测</span>
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
            重试
          </button>
        </div>
      </AdminAlert>

      <!-- 探测加载中：单行文案，避免与副标题重复造成“两个状态”观感 -->
      <div v-if="loading" class="flex flex-col items-center justify-center py-16 text-admin-text-3">
        <Refresh class="size-6 animate-spin text-admin-accent mb-2.5" />
        <p class="text-xs font-medium text-admin-text-2">正在比对社区模型目录...</p>
      </div>

      <!-- 结果区与 loading 互斥：必须包在 template v-else 里 -->
      <template v-else>
        <!-- 空结果提示：失败态只保留顶部 error alert（含重试），此处不再叠加 -->
        <div
          v-if="!error && filteredModels.length === 0"
          class="flex flex-col items-center justify-center py-14 text-admin-text-3 text-center"
        >
          <Cube class="size-8 text-admin-text-3/60 mb-2" />
          <p class="text-xs font-medium text-admin-text-2">
            {{ emptyDetectionTitle }}
          </p>
          <p class="text-[11px] text-admin-text-3 mt-1 max-w-sm">
            {{ emptyDetectionHint }}
          </p>
        </div>

        <!-- 紧凑 Checkbox 列表表格 -->
        <div
          v-else-if="!error"
          class="max-h-[380px] overflow-y-auto rounded-lg border border-admin-border-soft bg-white"
        >
          <table class="w-full text-left text-xs border-collapse">
            <thead
              class="sticky top-0 z-10 bg-admin-surface-muted/80 backdrop-blur-xs border-b border-admin-border-soft text-admin-text-2 font-medium"
            >
              <tr>
                <th class="w-10 px-3 py-2.5 text-center">
                  <input
                    type="checkbox"
                    class="size-3.5 rounded border-admin-border text-admin-accent focus:ring-admin-accent cursor-pointer"
                    :checked="isAllSelected"
                    :indeterminate="isIndeterminate"
                    :disabled="availableFilteredCount === 0"
                    aria-label="全选本页可用模型"
                    @change="toggleSelectAll"
                  />
                </th>
                <th class="px-3 py-2.5">模型信息</th>
                <th class="px-3 py-2.5 w-32">供应商</th>
                <th class="px-3 py-2.5 w-24 text-right">状态</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-admin-border-soft/60">
              <tr
                v-for="model in filteredModels"
                :key="model.id"
                class="transition-colors hover:bg-admin-surface-muted/40 cursor-pointer"
                :class="{
                  'bg-admin-accent/[0.03]': selectedModelIds.has(model.id),
                  'opacity-60 bg-admin-surface-muted/20 cursor-not-allowed': isModelExisting(
                    model.id,
                  ),
                }"
                @click="toggleRowSelection(model.id)"
              >
                <!-- Checkbox -->
                <td class="w-10 px-3 py-2.5 text-center" @click.stop>
                  <input
                    type="checkbox"
                    class="size-3.5 rounded border-admin-border text-admin-accent focus:ring-admin-accent cursor-pointer disabled:cursor-not-allowed"
                    :checked="selectedModelIds.has(model.id)"
                    :disabled="isModelExisting(model.id)"
                    :aria-label="`选择 ${model.name || model.id}`"
                    @change="toggleModelSelection(model.id)"
                  />
                </td>

                <!-- 模型信息 -->
                <td class="px-3 py-2.5">
                  <div class="flex items-center gap-2.5 min-w-0">
                    <div
                      class="flex size-6 shrink-0 items-center justify-center rounded-md overflow-hidden"
                      :class="
                        resolveModelBrandLogo(model)
                          ? 'bg-admin-surface-muted/60 border border-admin-border-soft/60'
                          : 'bg-admin-accent/10 text-admin-accent'
                      "
                    >
                      <img
                        v-if="resolveModelBrandLogo(model)"
                        :src="resolveModelBrandLogo(model)"
                        :alt="model.provider"
                        class="size-3.5 object-contain"
                        loading="lazy"
                      />
                      <Cube v-else class="size-3.5" />
                    </div>
                    <div class="flex flex-col gap-0.5 min-w-0">
                      <span class="font-medium text-admin-text-1 truncate">
                        {{ model.name || model.id }}
                      </span>
                      <span class="font-mono text-[11px] text-admin-text-3 truncate">
                        {{ model.id }}
                      </span>
                    </div>
                  </div>
                </td>

                <!-- 供应商 -->
                <td class="px-3 py-2.5">
                  <span
                    class="inline-flex items-center rounded-md px-1.5 py-0.5 text-[11px] font-medium"
                    :class="modelProviderBadgeClass(model.provider)"
                  >
                    {{ model.provider }}
                  </span>
                </td>

                <!-- 状态 -->
                <td class="px-3 py-2.5 text-right">
                  <span
                    v-if="isModelExisting(model.id)"
                    class="inline-flex items-center gap-1 rounded-full bg-admin-surface-muted px-2 py-0.5 text-[10px] font-medium text-admin-text-3"
                  >
                    已在库中
                  </span>
                  <span
                    v-else-if="selectedModelIds.has(model.id)"
                    class="inline-flex items-center gap-1 rounded-full bg-admin-accent/10 px-2 py-0.5 text-[10px] font-medium text-admin-accent"
                  >
                    <Check class="size-3" />
                    已勾选
                  </span>
                  <span
                    v-else
                    class="inline-flex items-center rounded-full bg-admin-border-soft/40 px-2 py-0.5 text-[10px] font-medium text-admin-text-3"
                  >
                    未添加
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
        <div class="text-xs text-admin-text-3">
          <span v-if="selectedCount > 0">已选 {{ selectedCount }} 个模型</span>
        </div>

        <div class="flex items-center gap-2">
          <AdminButton variant="outline" :disabled="pending" @click="emit('create')">
            手动新增
          </AdminButton>

          <AdminButton variant="secondary" :disabled="pending" @click="handleClose">
            取消
          </AdminButton>

          <AdminButton
            variant="primary"
            :disabled="selectedCount === 0 || pending"
            :loading="pending"
            @click="handleImportSelected"
          >
            导入所选模型{{ selectedCount > 0 ? ` (${selectedCount})` : '' }}
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

// 供应商下拉项
const providerOptions = computed(() => {
  const providers = Array.from(new Set(detectedModels.value.map((m) => m.provider))).filter(Boolean)
  return [{ label: '全部供应商', value: 'all' }, ...providers.map((p) => ({ label: p, value: p }))]
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
    error.value = err instanceof Error ? err.message : '探测网关模型失败'
  } finally {
    loading.value = false
  }
}

// 空结果文案：按后端元信息区分“未配置”与“扫过无可用”，不再统一显示空列表。
const emptyDetectionTitle = computed(() => {
  if (searchQuery.value.trim()) return '未找到匹配的模型'
  const meta = detectedMeta.value
  if (meta && meta.configured === 0) return '网关未配置任何 Provider'
  if (meta) return `已配置 ${meta.configured} 家 Provider，社区目录中未发现可用模型`
  return '未探测到可用的网关模型'
})

const emptyDetectionHint = computed(() => {
  const meta = detectedMeta.value
  if (meta && meta.configured === 0 && !searchQuery.value.trim()) {
    return '请先在 Cloudflare AI Gateway 控制台启用 Provider（BYOK 存密钥），再回来探测入库。'
  }
  return '请检查 Cloudflare AI Gateway 是否已配置相应 Provider 的 API Key (BYOK)。仍无结果请用下方「手动新增」入库。'
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
