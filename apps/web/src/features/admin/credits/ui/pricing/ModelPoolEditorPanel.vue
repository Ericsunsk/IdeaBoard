<template>
  <div class="flex flex-col gap-4">
    <div v-if="catalogLoading" class="text-xs text-muted-foreground">
      <span>{{ BILLING_PRICING_MESSAGES.poolCatalogLoading }}</span>
    </div>

    <ul v-if="draft.length > 0" class="flex flex-col gap-2">
      <li
        v-for="(entry, index) in draft"
        :key="entry.modelId"
        class="flex flex-col gap-2 rounded-panel border border-border p-3"
      >
        <div class="flex min-w-0 items-center gap-2">
          <span class="min-w-0 flex-1 truncate font-mono text-xs text-foreground">
            {{ entry.modelId }}
          </span>
          <AdminSwitch
            :model-value="entry.enabled"
            :disabled="pending"
            :aria-label="BILLING_PRICING_MESSAGES.poolEnabledAria(entry.modelId)"
            @update:model-value="setEnabled(index, $event)"
          />
          <ActionButton
            variant="outline"
            :disabled="pending"
            :aria-label="BILLING_PRICING_MESSAGES.poolRemoveAria(entry.modelId)"
            @click="removeEntry(index)"
          >
            <Xmark class="size-3.5" />
          </ActionButton>
        </div>
        <div class="flex items-center gap-1.5" role="group">
          <button
            v-for="resolution in POOL_RESOLUTIONS"
            :key="resolution"
            type="button"
            :disabled="pending"
            :aria-pressed="entry.resolutions.includes(resolution)"
            :aria-label="
              BILLING_PRICING_MESSAGES.poolResolutionAria(
                entry.modelId,
                resolution.toUpperCase(),
                entry.resolutions.includes(resolution),
              )
            "
            :class="resolutionBadgeClass(entry.resolutions.includes(resolution))"
            @click="toggleResolution(index, resolution)"
          >
            {{ resolution.toUpperCase() }}
          </button>
        </div>
        <!-- 错误行恒占一行高度：无错误时留空，避免校验出现时纵向重排。 -->
        <p class="min-h-4 text-xs text-destructive" role="alert">
          {{
            poolEntryErrorVisible(entry)
              ? BILLING_PRICING_MESSAGES.poolEnabledWithoutResolution
              : ''
          }}
        </p>
      </li>
    </ul>
    <p v-else class="text-xs text-muted-foreground">{{ BILLING_PRICING_MESSAGES.poolEmpty }}</p>

    <div class="flex items-center gap-2">
      <AdminSelect
        v-model="selectedCatalogId"
        :options="catalogSelectOptions"
        :placeholder="BILLING_PRICING_MESSAGES.poolAddPlaceholder"
        :aria-label="BILLING_PRICING_MESSAGES.poolAddPlaceholder"
        size="small"
        class="flex-1"
        :disabled="pending || availableCatalogModels.length === 0"
      />
      <ActionButton
        variant="outline"
        :disabled="pending || !selectedCatalogId"
        @click="addSelected"
      >
        {{ BILLING_PRICING_MESSAGES.poolAddModel }}
      </ActionButton>
    </div>
    <ActionButton
      v-if="catalogError"
      variant="outline"
      :disabled="pending || catalogLoading"
      @click="emit('reloadCatalog')"
    >
      <Refresh class="size-4" />
      {{ BILLING_PRICING_MESSAGES.poolReload }}
    </ActionButton>
    <p v-else-if="availableCatalogModels.length === 0" class="text-xs text-muted-foreground">
      {{ BILLING_PRICING_MESSAGES.poolNoCatalogModels }}
    </p>

    <ActionButton v-if="stale" variant="outline" :disabled="pending" @click="emit('reload')">
      <Refresh class="size-4" />
      {{ BILLING_PRICING_MESSAGES.poolReload }}
    </ActionButton>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Refresh, Xmark } from '@iconoir/vue'
import { AdminSelect } from '../../../shared'
import ActionButton from '@/core/components/ActionButton.vue'
import { AdminSwitch } from '../../../shared'
import { OUTPUT_RESOLUTION_OPTIONS } from '@/core/contracts/generationContract'
import type {
  AdminModelPoolEntry,
  AdminPoolCatalogModel,
  PoolResolution,
} from '../../core/pricing/billableCatalogTypes'
import { BILLING_PRICING_MESSAGES } from './messages'

const props = withDefaults(
  defineProps<{
    entries?: AdminModelPoolEntry[] | undefined
    catalogModels?: AdminPoolCatalogModel[] | undefined
    catalogLoading?: boolean | undefined
    pending?: boolean | undefined
    stale?: boolean | undefined
    saveDisabled?: boolean | undefined
    catalogError?: string | undefined
  }>(),
  {
    entries: () => [],
    catalogModels: () => [],
    catalogLoading: false,
    pending: false,
    stale: false,
    saveDisabled: false,
    catalogError: '',
  },
)

const emit = defineEmits<{
  reload: []
  reloadCatalog: []
}>()

const POOL_RESOLUTIONS: readonly PoolResolution[] = OUTPUT_RESOLUTION_OPTIONS

const draft = ref<AdminModelPoolEntry[]>([])
const selectedCatalogId = ref('')
const showError = ref(false)

watch(
  () => props.entries,
  (entries) => {
    draft.value = (entries ?? []).map((entry) => ({
      modelId: entry.modelId,
      enabled: entry.enabled,
      resolutions: [...entry.resolutions],
    }))
    selectedCatalogId.value = ''
    showError.value = false
  },
  { immediate: true, deep: true },
)

const availableCatalogModels = computed(() => {
  const taken = new Set(draft.value.map((entry) => entry.modelId.toLowerCase()))
  return (props.catalogModels ?? []).filter((model) => !taken.has(model.id.toLowerCase()))
})

// AdminSelect 取 label/value 结构：保留原生 select 的 `name || id` 回退，
// 空 name 的目录项仍显示可辨识的 id。
const catalogSelectOptions = computed(() =>
  availableCatalogModels.value.map((model) => ({
    label: model.name || model.id,
    value: model.id,
  })),
)

function setEnabled(index: number, enabled: boolean) {
  const entry = draft.value[index]
  if (entry) entry.enabled = enabled
}

function poolEntryErrorVisible(entry: AdminModelPoolEntry): boolean {
  return showError.value && entry.enabled && entry.resolutions.length === 0
}

function toggleResolution(index: number, resolution: PoolResolution) {
  const entry = draft.value[index]
  if (!entry) return
  const at = entry.resolutions.indexOf(resolution)
  if (at >= 0) entry.resolutions.splice(at, 1)
  else entry.resolutions.push(resolution)
  entry.resolutions.sort()
}

function removeEntry(index: number) {
  draft.value.splice(index, 1)
}

function addSelected() {
  const id = selectedCatalogId.value.trim()
  if (!id) return
  if (draft.value.some((entry) => entry.modelId.toLowerCase() === id.toLowerCase())) return
  draft.value.push({ modelId: id, enabled: true, resolutions: [] })
  selectedCatalogId.value = ''
}

function resolutionBadgeClass(on: boolean): string {
  return on
    ? 'rounded-control px-1.5 py-0.5 font-mono text-admin-caption font-bold text-foreground bg-primary-faint-strong ring-1 ring-inset ring-primary-line-strong cursor-pointer'
    : 'rounded-control px-1.5 py-0.5 font-mono text-admin-caption text-muted-foreground bg-muted cursor-pointer'
}

function checkPoolValidity(): boolean {
  showError.value = true
  if (props.saveDisabled) return false
  if (draft.value.some((entry) => entry.enabled && entry.resolutions.length === 0)) return false
  return true
}

function getDraftEntries(): AdminModelPoolEntry[] {
  return draft.value.map((entry) => ({
    modelId: entry.modelId,
    enabled: entry.enabled,
    resolutions: [...entry.resolutions],
  }))
}

defineExpose({
  checkPoolValidity,
  getDraftEntries,
})
</script>
