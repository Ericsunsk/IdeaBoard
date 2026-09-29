<template>
  <AdminDialogShell
    :visible="visible"
    :header="BILLING_PRICING_MESSAGES.poolDialogTitle(item?.displayName)"
    :description="BILLING_PRICING_MESSAGES.poolDialogDescription"
    size="form"
    content-layout="plain"
    :busy="pending"
    @update:visible="emit('update:visible', $event)"
  >
    <div class="flex flex-col gap-4">
      <div class="flex items-center justify-between text-xs text-admin-text-3">
        <span>{{ BILLING_PRICING_MESSAGES.poolVersion(version) }}</span>
        <span v-if="catalogLoading">{{ BILLING_PRICING_MESSAGES.poolCatalogLoading }}</span>
      </div>

      <ul v-if="draft.length > 0" class="flex flex-col gap-2">
        <li
          v-for="(entry, index) in draft"
          :key="entry.modelId"
          class="flex flex-col gap-2 rounded-xl border border-admin-border-soft p-3"
        >
          <div class="flex min-w-0 items-center gap-2">
            <span class="min-w-0 flex-1 truncate font-mono text-xs text-admin-text-1">
              {{ entry.modelId }}
            </span>
            <AdminSwitch
              :model-value="entry.enabled"
              :disabled="pending"
              :aria-label="BILLING_PRICING_MESSAGES.poolEnabledAria(entry.modelId)"
              @update:model-value="setEnabled(index, $event)"
            />
            <AdminButton
              size="small"
              variant="outline"
              :class="adminActionButtonClass"
              :disabled="pending"
              :aria-label="BILLING_PRICING_MESSAGES.poolRemoveAria(entry.modelId)"
              @click="removeEntry(index)"
            >
              <Xmark class="size-3.5" />
            </AdminButton>
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
          <p
            v-if="showError && entry.enabled && entry.resolutions.length === 0"
            class="text-xs text-admin-danger"
            role="alert"
          >
            {{ BILLING_PRICING_MESSAGES.poolEnabledWithoutResolution }}
          </p>
        </li>
      </ul>
      <p v-else class="text-xs text-admin-text-3">{{ BILLING_PRICING_MESSAGES.poolEmpty }}</p>

      <div class="flex items-center gap-2">
        <select
          v-model="selectedCatalogId"
          class="h-8 min-w-0 flex-1 rounded-lg border border-admin-border-soft bg-white px-2 text-xs text-admin-text-1"
          :aria-label="BILLING_PRICING_MESSAGES.poolAddPlaceholder"
          :disabled="pending || availableCatalogModels.length === 0"
        >
          <option value="" disabled>{{ BILLING_PRICING_MESSAGES.poolAddPlaceholder }}</option>
          <option v-for="model in availableCatalogModels" :key="model.id" :value="model.id">
            {{ model.name || model.id }}
          </option>
        </select>
        <AdminButton
          size="small"
          variant="outline"
          :class="adminActionButtonClass"
          :disabled="pending || !selectedCatalogId"
          @click="addSelected"
        >
          {{ BILLING_PRICING_MESSAGES.poolAddModel }}
        </AdminButton>
      </div>
      <AdminAlert v-if="catalogError" severity="error">
        {{ catalogError }}
      </AdminAlert>
      <AdminButton
        v-if="catalogError"
        variant="outline"
        :class="adminActionButtonClass"
        :disabled="pending || catalogLoading"
        @click="emit('reloadCatalog')"
      >
        <Refresh class="size-4" />
        {{ BILLING_PRICING_MESSAGES.poolReload }}
      </AdminButton>
      <p v-else-if="availableCatalogModels.length === 0" class="text-xs text-admin-text-3">
        {{ BILLING_PRICING_MESSAGES.poolNoCatalogModels }}
      </p>

      <AdminAlert v-if="errorMessage" severity="error">
        {{ errorMessage }}
      </AdminAlert>
      <AdminButton
        v-if="stale"
        variant="outline"
        :class="adminActionButtonClass"
        :disabled="pending"
        @click="emit('reload')"
      >
        <Refresh class="size-4" />
        {{ BILLING_PRICING_MESSAGES.poolReload }}
      </AdminButton>
    </div>

    <template #footer>
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        :disabled="pending"
        @click="close"
      >
        {{ BILLING_PRICING_MESSAGES.cancel }}
      </AdminButton>
      <AdminButton
        :class="adminActionButtonClass"
        :loading="pending"
        :disabled="pending || saveDisabled"
        @click="submit"
      >
        {{ BILLING_PRICING_MESSAGES.save }}
      </AdminButton>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Refresh, Xmark } from '@iconoir/vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import { adminActionButtonClass } from '../../shared/lib/uiClasses'
import type {
  AdminBillableItemCard,
  AdminModelPoolEntry,
  AdminPoolCatalogModel,
  PoolResolution,
} from './billableCatalogTypes'
import { BILLING_PRICING_MESSAGES } from './messages'

const props = withDefaults(
  defineProps<{
    visible: boolean
    item?: AdminBillableItemCard | null
    version?: number | undefined
    entries?: AdminModelPoolEntry[] | undefined
    catalogModels?: AdminPoolCatalogModel[] | undefined
    catalogLoading?: boolean | undefined
    pending?: boolean | undefined
    errorMessage?: string | undefined
    stale?: boolean | undefined
    saveDisabled?: boolean | undefined
    catalogError?: string | undefined
  }>(),
  {
    item: null,
    version: 0,
    entries: () => [],
    catalogModels: () => [],
    catalogLoading: false,
    pending: false,
    errorMessage: '',
    stale: false,
    saveDisabled: false,
    catalogError: '',
  },
)

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  submit: [entries: AdminModelPoolEntry[]]
  reload: []
  reloadCatalog: []
}>()

const POOL_RESOLUTIONS: readonly PoolResolution[] = ['1k', '2k', '4k']

const draft = ref<AdminModelPoolEntry[]>([])
const selectedCatalogId = ref('')
const showError = ref(false)

// The dialog edits a local draft: opening (re)seeds it from the latest
// pool snapshot so a stale base version can never be saved silently.
// The pool has no priority, fallback, or weight fields by contract.
watch(
  () => [props.visible, props.entries] as const,
  ([visible]) => {
    if (!visible) return
    draft.value = (props.entries ?? []).map((entry) => ({
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

function setEnabled(index: number, enabled: boolean) {
  const entry = draft.value[index]
  if (entry) entry.enabled = enabled
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
    ? 'rounded px-1.5 py-0.5 font-mono text-admin-caption font-bold text-admin-title-color bg-admin-accent/15 ring-1 ring-inset ring-admin-accent/40 cursor-pointer'
    : 'rounded px-1.5 py-0.5 font-mono text-admin-caption text-admin-text-3 bg-admin-surface-muted cursor-pointer'
}

function close() {
  if (!props.pending) emit('update:visible', false)
}

function submit() {
  showError.value = true
  if (props.pending) return
  // Client-side guard mirrors the server rule: an enabled Model must
  // support at least one resolution before save.
  if (draft.value.some((entry) => entry.enabled && entry.resolutions.length === 0)) return
  emit(
    'submit',
    draft.value.map((entry) => ({
      modelId: entry.modelId,
      enabled: entry.enabled,
      resolutions: [...entry.resolutions],
    })),
  )
}
</script>
