<template>
  <AdminDialogShell
    :visible="visible"
    :header="MODELS_MESSAGES.addHeader"
    :description="MODELS_MESSAGES.addDescription"
    size="compact"
    @update:visible="emit('update:visible', $event)"
    @close="handleClose"
  >
    <div class="flex flex-col gap-4">
      <div
        v-if="loading"
        class="flex flex-col items-center justify-center py-16 text-muted-foreground"
      >
        <Refresh class="size-6 animate-spin text-primary mb-2.5" />
        <p class="text-xs font-medium text-muted-foreground">
          {{ MODELS_MESSAGES.addProviderLoading }}
        </p>
      </div>

      <div
        v-else-if="loadError"
        class="flex flex-col items-center justify-center gap-2 py-14 text-center text-muted-foreground"
      >
        <p class="text-xs font-medium text-destructive">{{ loadError }}</p>
        <AdminButton variant="outline" @click="void loadProviders()">{{
          MODELS_MESSAGES.retry
        }}</AdminButton>
      </div>

      <div v-else class="flex flex-col gap-4">
        <div
          v-if="providers.length === 0"
          class="flex flex-col items-center justify-center py-14 text-center text-muted-foreground"
        >
          <Cube class="size-8 text-muted-foreground/60 mb-2" />
          <p class="text-xs font-medium text-muted-foreground">
            {{ MODELS_MESSAGES.addProviderEmpty }}
          </p>
          <p class="text-admin-caption mt-1 max-w-sm">{{ MODELS_MESSAGES.addHintEnableFirst }}</p>
        </div>

        <template v-else>
          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-medium text-foreground" for="add-provider-select">
              {{ MODELS_MESSAGES.addProviderLabel }}
            </label>
            <AdminSelect
              id="add-provider-select"
              v-model="selectedProviderSlug"
              :options="providerOptions"
              size="small"
              :placeholder="MODELS_MESSAGES.addProviderPlaceholder"
            />
            <p v-if="hasUnadaptedProvider" class="text-admin-caption text-muted-foreground">
              {{ MODELS_MESSAGES.addProviderUnadaptedHint }}
            </p>
          </div>
          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-medium text-foreground" for="add-model-input">
              {{ MODELS_MESSAGES.addModelLabel }}
            </label>
            <input
              id="add-model-input"
              v-model="modelId"
              type="text"
              class="h-8 rounded-control border border-input bg-card px-2.5 text-xs text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
              :placeholder="MODELS_MESSAGES.addModelPlaceholder"
            />
            <p class="text-admin-caption text-muted-foreground">
              {{ MODELS_MESSAGES.addModelHint }}
            </p>
          </div>

          <div
            v-if="providerPrefixError || duplicateError"
            class="text-xs text-destructive"
            role="alert"
          >
            {{ providerPrefixError || duplicateError }}
          </div>
        </template>
      </div>
    </div>

    <template #footer>
      <div class="flex w-full items-center justify-between">
        <div class="text-xs text-muted-foreground" />
        <div class="flex items-center gap-2">
          <AdminButton variant="outline" :disabled="pending" @click="emit('create')">
            {{ MODELS_MESSAGES.addManual }}
          </AdminButton>
          <AdminButton variant="secondary" :disabled="pending" @click="handleClose">
            {{ MODELS_MESSAGES.cancel }}
          </AdminButton>
          <AdminButton variant="primary" :disabled="!canImport || pending" @click="handleImport">
            {{ MODELS_MESSAGES.addImport }}
          </AdminButton>
        </div>
      </div>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Cube, Refresh } from '@iconoir/vue'
import { AdminButton } from '../../../shared'
import { AdminDialogShell } from '../../../shared'
import { AdminSelect } from '../../../shared'
import { createAdminGatewayResource, type GatewayProviderOption } from '../../resource'
import { useAdminModelCatalogFeedback } from '../../session/useAdminModelCatalogFeedback'
import { formatProviderName, modelIdentityKey, type AdminModelItem } from '../../core/catalogData'
import { buildModelItemFromManual, hasModelProviderPrefix } from '../../core/modelForm'
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
  // 高级新增：空 EditDialog 表单入库（完整字段）。
  (e: 'create'): void
}>()

const catalogResource = createAdminGatewayResource().catalog
const feedback = useAdminModelCatalogFeedback()

const providers = ref<GatewayProviderOption[]>([])
const loading = ref(false)
const loadError = ref('')
const selectedProviderSlug = ref('')
const modelId = ref('')

const existingKeySet = computed(() => new Set(props.existingModels.map(modelIdentityKey)))

// Worker has no adapter for this slug when false: the row renders disabled
// with a 缺适配器 hint and can never be picked. Absent (legacy payloads and
// catalog-fallback rows) means available.
function isUnadaptedProvider(p: GatewayProviderOption): boolean {
  return p.available === false
}

function firstPickableProviderSlug(rows: GatewayProviderOption[]): string {
  // Unadapted rows are disabled and can never be picked: default to the
  // first available one, falling back to the first row so the hint stays
  // visible even when nothing is pickable.
  return rows.find((p) => !isUnadaptedProvider(p))?.slug ?? rows[0]!.slug
}

const providerOptions = computed(() =>
  providers.value.map((p) => {
    const base = p.display_name?.trim() ? p.display_name : formatProviderName(p.slug)
    const isUnadapted = isUnadaptedProvider(p)
    return {
      label: isUnadapted ? `${base}${MODELS_MESSAGES.addProviderUnadaptedSuffix}` : base,
      value: p.slug,
      // eslint-disable-next-line governance/boolean-prefix -- Reka `SelectItem` contract prop.
      disabled: isUnadapted,
    }
  }),
)

const hasUnadaptedProvider = computed(() => providers.value.some(isUnadaptedProvider))

const selectedProvider = computed(() =>
  providers.value.find((p) => p.slug === selectedProviderSlug.value),
)

// 原生模型名可含 vendor/model；只拒绝重复的所选供应商前缀。
const providerPrefixError = computed(() => {
  const id = modelId.value.trim()
  if (!hasModelProviderPrefix(selectedProviderSlug.value, id)) return ''
  return MODELS_MESSAGES.addProviderPrefixError
})

const duplicateError = computed(() => {
  const id = modelId.value.trim()
  if (!id || providerPrefixError.value) return ''
  const key = modelIdentityKey({ provider: selectedProviderSlug.value, id })
  if (existingKeySet.value.has(key)) return MODELS_MESSAGES.addDuplicate
  return ''
})

const canImport = computed(() => {
  if (loading.value || loadError.value) return false
  if (!selectedProvider.value) return false
  // Unadapted rows are disabled in the dropdown; a stale selection pointing
  // at one (list refreshed while the dialog stayed open) must not import.
  if (isUnadaptedProvider(selectedProvider.value)) return false
  const id = modelId.value.trim()
  if (!id) return false
  return duplicateError.value === '' && providerPrefixError.value === ''
})

async function loadProviders() {
  loading.value = true
  loadError.value = ''
  try {
    providers.value = await catalogResource.listProviders()
    if (!selectedProviderSlug.value && providers.value.length > 0) {
      selectedProviderSlug.value = firstPickableProviderSlug(providers.value)
    }
  } catch (caughtError) {
    loadError.value =
      caughtError instanceof Error ? caughtError.message : MODELS_MESSAGES.addProviderLoadFailed
    try {
      feedback.listFailed(caughtError)
    } catch {
      // feedback best-effort
    }
  } finally {
    loading.value = false
  }
}

watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      modelId.value = ''
      loadError.value = ''
      if (providers.value.length === 0) {
        void loadProviders()
      } else if (!selectedProviderSlug.value && providers.value.length > 0) {
        selectedProviderSlug.value = firstPickableProviderSlug(providers.value)
      }
    }
  },
  { immediate: true },
)

function handleClose() {
  emit('update:visible', false)
}

function handleImport() {
  const provider = selectedProvider.value
  const id = modelId.value.trim()
  if (!provider || isUnadaptedProvider(provider)) return
  if (!id || duplicateError.value || providerPrefixError.value) return
  emit('import', [buildModelItemFromManual(provider, id)])
}
</script>
