<template>
  <div :class="adminDashboardRootClass">
    <!-- 1. 顶栏操作与搜索过滤工具条 (1:1 复刻功能定价页规范) -->
    <div class="flex w-full flex-col gap-3">
      <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
        <!-- 左侧：搜索输入框与状态筛选器 -->
        <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <AdminToolbarSearch
            v-model="searchKeyword"
            fluid
            :input-label="PROMPTS_MESSAGES.searchLabel"
            :placeholder="PROMPTS_MESSAGES.searchPlaceholder"
          />

          <AdminOptionFilter
            v-model="statusFilter"
            :options="statusFilterOptions"
            all-value="all"
            :label="PROMPTS_MESSAGES.filterLabel"
          />
        </div>

        <!-- 右侧：刷新操作 -->
        <div class="flex shrink-0 items-center gap-2">
          <AdminRefreshButton
            :is-loading="isLoading"
            :aria-label="PROMPTS_MESSAGES.refresh"
            @click="loadPrompts()"
          />
        </div>
      </div>
    </div>

    <!-- 2. 空状态提示 (无数据或过滤未命中) -->
    <AdminEmptyState
      v-if="filteredFeatureGroups.length === 0"
      :title="!hasActiveFilter ? PROMPTS_MESSAGES.emptyNoConfig : PROMPTS_MESSAGES.emptyNoMatch"
      :description="
        !hasActiveFilter ? PROMPTS_MESSAGES.emptyNoConfigHint : PROMPTS_MESSAGES.emptyNoMatchHint
      "
      :icon="!hasActiveFilter ? Spark : Search"
    >
      <template #actions>
        <button
          v-if="hasActiveFilter"
          type="button"
          class="flex h-8 items-center gap-1.5 rounded-control border border-border bg-card px-3 text-xs font-medium text-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:bg-muted cursor-pointer"
          @click="clearFilters"
        >
          <Xmark class="size-3.5" />
          <span>{{ PROMPTS_MESSAGES.clearFilters }}</span>
        </button>
      </template>
    </AdminEmptyState>

    <!-- 3. 分组表格视图 (按 Image / Prompt / Fusion 分组展示，1:1 对齐功能定价页 BillingPricingTable 规范) -->
    <div v-else class="flex flex-col gap-6">
      <section
        v-for="group in filteredFeatureGroups"
        :key="group.key"
        :aria-label="PROMPTS_MESSAGES.groupAria(group.title)"
      >
        <!-- 分组标题栏 (1:1 对齐 BillingPricingTable section header) -->
        <div class="mb-2 flex items-center">
          <h3 class="text-sm font-bold text-foreground">{{ group.title }}</h3>
        </div>

        <!-- 数据表格壳体 -->
        <div :class="adminTableCardClass">
          <AdminTable
            :value="group.items"
            :is-loading="isLoading"
            data-key="slug"
            scrollable
            :frame-class="adminTableFrameClass"
            :table-style="adminTableStyle"
            :class="adminTableClass"
          >
            <template #empty>
              <div class="p-6 text-center text-xs text-muted-foreground">
                {{ PROMPTS_MESSAGES.groupEmpty }}
              </div>
            </template>

            <!-- 1. Service (1:1 对齐功能定价页 Service 规范) -->
            <AdminColumn field="service" header="Service" :style="{ width: '200px' }">
              <template #body="{ data }">
                <AdminServiceCell :service="data.slug" />
              </template>
            </AdminColumn>

            <!-- 2. Status（方案 A：载体统一 StatusBadge） -->
            <AdminColumn field="status" header="Status" :style="{ width: '105px' }">
              <template #body="{ data }">
                <StatusBadge
                  v-if="rowView(data.slug).status !== 'unsupported'"
                  :label="
                    rowView(data.slug).status === 'configured'
                      ? PROMPTS_MESSAGES.configured
                      : PROMPTS_MESSAGES.unconfigured
                  "
                  :tone="rowView(data.slug).status === 'configured' ? 'success' : 'info'"
                  size="small"
                />
                <span v-else :class="adminTableMutedValueClass">—</span>
              </template>
            </AdminColumn>

            <!-- 3. 可用变量 (Variables) -->
            <AdminColumn
              field="variables"
              :header="PROMPTS_MESSAGES.variablesColumn"
              :style="{ width: '220px', minWidth: '180px' }"
            >
              <template #body="{ data }">
                <span
                  v-if="rowView(data.slug).status === 'unsupported'"
                  :class="adminTableMutedValueClass"
                  >—</span
                >
                <div v-else class="flex flex-wrap items-center gap-1">
                  <span
                    v-for="variable in rowView(data.slug).vars"
                    :key="variable.key"
                    class="inline-flex items-center rounded-control bg-muted/80 px-1.5 py-0.5 font-mono text-admin-micro font-medium text-muted-foreground"
                  >
                    {{ variable.key }}
                  </span>
                  <span
                    v-if="rowView(data.slug).vars.length === 0"
                    class="text-xs text-muted-foreground font-mono"
                  >
                    {{ PROMPTS_MESSAGES.noVariables }}
                  </span>
                </div>
              </template>
            </AdminColumn>

            <!-- 4. 提示词内容预览 (Prompt Preview, 只读已保存值, draft 仅弹窗内可见) -->
            <AdminColumn field="prompt" :header="PROMPTS_MESSAGES.previewColumn">
              <template #body="{ data }">
                <span
                  v-if="rowView(data.slug).status === 'unsupported'"
                  :class="adminTableMutedValueClass"
                  >—</span
                >
                <div
                  v-else-if="rowView(data.slug).saved?.trim()"
                  class="line-clamp-1 max-w-xl font-mono text-xs text-muted-foreground"
                  :title="rowView(data.slug).saved"
                >
                  {{ rowView(data.slug).saved }}
                </div>
                <span v-else class="text-xs text-muted-foreground font-mono">{{
                  PROMPTS_MESSAGES.unset
                }}</span>
              </template>
            </AdminColumn>

            <!-- 5. 操作 (Actions, 1:1 对齐功能定价表规范) -->
            <AdminColumn
              field="actions"
              :header="PROMPTS_MESSAGES.actionsColumn"
              :style="{ width: '120px' }"
            >
              <template #body="{ data }">
                <span
                  v-if="rowView(data.slug).status === 'unsupported'"
                  :class="adminTableMutedValueClass"
                  >—</span
                >
                <div v-else class="flex items-center gap-2">
                  <ActionButton
                    :aria-label="PROMPTS_MESSAGES.configureAria(data.title)"
                    :disabled="isInitialLoading"
                    @click="openEditor(data)"
                  >
                    {{
                      rowView(data.slug).status === 'configured'
                        ? PROMPTS_MESSAGES.adjust
                        : PROMPTS_MESSAGES.setup
                    }}
                  </ActionButton>
                </div>
              </template>
            </AdminColumn>
          </AdminTable>
        </div>
      </section>
    </div>

    <!-- 5. 提示词编辑弹窗 -->
    <PromptConfigEditorDialog
      :item="activeItem"
      :draft="activeItem ? (promptMap[activeItem.slug] ?? '') : ''"
      :validation="activeValidation"
      :is-saving="Boolean(activeItem && isSaving[activeItem.slug])"
      :variables="activeVars"
      :header="editorHeader"
      @update:draft="onDraftInput"
      @save="onEditorSave"
      @close="closeEditor"
      @insert-variable="onInsertVariable"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Search, Spark, Xmark } from '@iconoir/vue'
import { usePromptConfigFeedback } from '../../session/usePromptConfigFeedback'
import { AdminEmptyState } from '../../../shared'
import ActionButton from '@/core/components/ActionButton.vue'
import { AdminTable } from '../../../shared'
import { AdminColumn } from '../../../shared'
import { AdminServiceCell } from '../../../shared'
import { AdminOptionFilter } from '../../../shared'
import { AdminRefreshButton } from '../../../shared'
import { StatusBadge } from '../../../shared'
import { AdminToolbarSearch } from '../../../shared'
import PromptConfigEditorDialog from '../editors/PromptConfigEditorDialog.vue'
import { usePromptConfigFilters } from '../../core/filtering'
import {
  adminDashboardRootClass,
  adminTableCardClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableStyle,
  adminTableMutedValueClass,
} from '../../../shared'
import { useAdminPromptConfigWorkspace } from '../../session/workspace'
import type { FeaturePromptVariable } from '../../session/workspace'
import {
  featureMap,
  getFeaturePromptStatus,
  groupedFeatureGroups,
  type FeatureGroup,
  type FeatureItem,
  type FeaturePromptStatus,
  type FeatureSlug,
} from '../../core/featurePromptMeta'
import { PROMPTS_MESSAGES } from './messages'

const feedback = usePromptConfigFeedback()
const promptConfigWorkspace = useAdminPromptConfigWorkspace({
  feedback,
  getFeatureTitle: (slug) => featureMap[slug as FeatureSlug]?.title ?? slug,
})
const {
  getDraftValidation,
  getPromptVariables,
  getSavedPrompt,
  insertVariableIntoDraft,
  loadPrompts,
  isLoading,
  promptMap,
  prompts,
  resetDrafts,
  savePrompt,
  isSaving,
} = promptConfigWorkspace

const activeItem = ref<FeatureItem | null>(null)

function formatServiceName(slug: string): string {
  if (!slug) return ''
  return slug.charAt(0).toUpperCase() + slug.slice(1)
}

const editorHeader = computed(() => {
  if (!activeItem.value) return ''
  return PROMPTS_MESSAGES.editorHeader(formatServiceName(activeItem.value.slug))
})

function isConfigured(slug: FeatureSlug): boolean {
  return getSavedPrompt(slug).trim().length > 0
}

const featureGroups = computed<FeatureGroup[]>(() =>
  groupedFeatureGroups.filter((group) => group.items.length > 0),
)

const {
  searchKeyword,
  statusFilter,
  statusFilterOptions,
  hasActiveFilter,
  clearFilters,
  filteredFeatureGroups,
} = usePromptConfigFilters({
  groups: featureGroups,
  isConfigured,
  getSavedPrompt,
  getPromptVariables,
})

type PromptRowView = {
  saved: string
  status: FeaturePromptStatus
  vars: readonly FeaturePromptVariable[]
}

const rowViewMap = computed(() => {
  const map: Record<string, PromptRowView> = {}
  for (const group of featureGroups.value) {
    for (const item of group.items) {
      const saved = getSavedPrompt(item.slug)
      map[item.slug] = {
        saved,
        status: getFeaturePromptStatus(item.slug, saved.trim().length > 0),
        vars: getVarsForSlug(item.slug),
      }
    }
  }
  return map
})

const activeVars = computed<readonly FeaturePromptVariable[]>(() =>
  activeItem.value ? getVarsForSlug(activeItem.value.slug) : [],
)

const activeValidation = computed(() =>
  activeItem.value ? getDraftValidation(activeItem.value.slug) : null,
)

// 模板索引安全：map 恒有全量 slug，fallback 仅满足 noUncheckedIndexedAccess。
const emptyRowView: PromptRowView = {
  saved: '',
  status: 'unconfigured',
  vars: [] as readonly FeaturePromptVariable[],
}

function rowView(slug: string): typeof emptyRowView {
  return rowViewMap.value[slug] ?? emptyRowView
}

function openEditor(item: FeatureItem) {
  if (rowView(item.slug).status === 'unsupported') return
  resetDrafts([item.slug])
  activeItem.value = item
}

function closeEditor() {
  if (activeItem.value) {
    resetDrafts([activeItem.value.slug])
  }
  activeItem.value = null
}

function onDraftInput(value: string) {
  if (activeItem.value) {
    promptMap[activeItem.value.slug] = value
  }
}

async function onEditorSave() {
  if (!activeItem.value) return
  const result = await savePrompt(activeItem.value.slug)
  if (result) {
    activeItem.value = null
  }
}

function onInsertVariable(payload: { key: string; position: number }) {
  if (!activeItem.value) return
  insertVariableIntoDraft(activeItem.value.slug, payload.key, payload.position)
}

// 初载：数据到达前不渲染 0 值计数，避免闪动误导。
const isInitialLoading = computed(() => isLoading.value && prompts.value.length === 0)

function getVarsForSlug(slug: FeatureSlug): readonly FeaturePromptVariable[] {
  return getPromptVariables(slug)
}
</script>
