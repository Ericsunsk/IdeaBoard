<template>
  <div class="flex flex-col gap-4">
    <div v-if="catalogLoading" class="text-xs text-muted-foreground">
      <span>{{ BILLING_PRICING_MESSAGES.poolCatalogLoading }}</span>
    </div>

    <ul v-if="draft.length > 0" class="flex flex-col gap-2">
      <li
        v-for="(entry, index) in draft"
        :key="displayId(entry)"
        class="flex flex-col gap-2 rounded-panel border border-border p-3"
      >
        <div class="flex min-w-0 items-center gap-2">
          <span class="min-w-0 flex-1 truncate font-mono text-xs text-foreground">
            {{ displayId(entry) }}
          </span>
          <AdminSwitch
            :model-value="entry.enabled"
            :disabled="pending"
            :aria-label="BILLING_PRICING_MESSAGES.poolEnabledAria(displayId(entry))"
            @update:model-value="setEnabled(index, $event)"
          />
          <ActionButton
            variant="outline"
            :disabled="pending"
            :aria-label="BILLING_PRICING_MESSAGES.poolRemoveAria(displayId(entry))"
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
                displayId(entry),
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
        v-model="selectedCatalogKey"
        :options="catalogSelectOptions"
        :placeholder="BILLING_PRICING_MESSAGES.poolAddPlaceholder"
        :aria-label="BILLING_PRICING_MESSAGES.poolAddPlaceholder"
        size="small"
        class="flex-1"
        :disabled="pending || availableCatalogModels.length === 0"
      />
      <ActionButton
        variant="outline"
        :disabled="pending || !selectedCatalogKey"
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
import { formatPoolModelId as displayId } from '../../core/pricing/billableCatalogTypes'
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
const selectedCatalogKey = ref('')
const showError = ref(false)

watch(
  () => props.entries,
  (entries) => {
    draft.value = (entries ?? []).map((entry) => ({
      provider: entry.provider,
      modelId: entry.modelId,
      enabled: entry.enabled,
      resolutions: [...entry.resolutions],
    }))
    selectedCatalogKey.value = ''
    showError.value = false
  },
  { immediate: true, deep: true },
)

function entryKey(provider: string, modelId: string): string {
  return `${provider.trim().toLowerCase()}/${modelId.trim().toLowerCase()}`
}

function catalogDisplayId(item: Pick<AdminPoolCatalogModel, 'id' | 'provider'>): string {
  return displayId({ provider: item.provider, modelId: item.id })
}

const availableCatalogModels = computed(() => {
  const taken = new Set(draft.value.map((entry) => entryKey(entry.provider, entry.modelId)))
  return (props.catalogModels ?? []).filter(
    (model) => !taken.has(entryKey(model.provider, model.id)),
  )
})

// AdminSelect 取 label/value 结构：保留原生 select 的 `name || id` 回退，
// 目录拼接（provider/model）优先展示，名称不同时附带。
const catalogSelectOptions = computed(() =>
  availableCatalogModels.value.map((model) => {
    const display = catalogDisplayId(model)
    return {
      label: model.name && model.name !== display ? `${display} · ${model.name}` : display,
      value: entryKey(model.provider, model.id),
    }
  }),
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
  const item = availableCatalogModels.value.find(
    (model) => entryKey(model.provider, model.id) === selectedCatalogKey.value,
  )
  if (!item) return
  draft.value.push({
    provider: item.provider,
    modelId: item.id.trim(),
    enabled: true,
    resolutions: [],
  })
  selectedCatalogKey.value = ''
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
    provider: entry.provider,
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
