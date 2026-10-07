<template>
  <SettingEditorCard
    :title="M.supportTitle"
    :status="status"
    :status-severity="query.error.value ? 'danger' : 'secondary'"
  >
    <AdminAlert v-if="query.error.value" severity="error">
      {{ M.supportLoadFailed }}
      <AdminButton variant="outline" @click="query.refetch()">{{ M.retry }}</AdminButton>
    </AdminAlert>
    <p v-else-if="loading" class="text-admin-body text-muted-foreground">{{ M.providerLoading }}</p>
    <p v-else-if="!form.channels.length" class="text-admin-body text-muted-foreground">
      {{ M.supportEmpty }}
    </p>
    <div
      v-for="(channel, index) in form.channels"
      :key="channel.id"
      class="grid min-w-0 gap-4 border-b border-border pb-4 md:grid-cols-2"
    >
      <AdminFormField :label="M.supportName" :for="`support-name-${channel.id}`">
        <AdminInput
          :id="`support-name-${channel.id}`"
          v-model="channel.name"
          :disabled="disabled"
          :maxlength="80"
          class="w-full"
        />
      </AdminFormField>
      <AdminFormField :label="M.supportKind" :for="`support-kind-${channel.id}`">
        <AdminSelect
          :id="`support-kind-${channel.id}`"
          v-model="channel.kind"
          :options="kindOptions"
          :disabled="disabled"
          class="w-full"
        />
      </AdminFormField>
      <AdminFormField
        :label="M.supportUrl"
        :for="`support-url-${channel.id}`"
        class="md:col-span-2"
      >
        <AdminInput
          :id="`support-url-${channel.id}`"
          v-model="channel.url"
          type="url"
          :disabled="disabled"
          :maxlength="2048"
          class="w-full"
        />
      </AdminFormField>
      <div class="flex flex-wrap items-center justify-between gap-3 md:col-span-2">
        <div class="flex items-center gap-2">
          <AdminSwitch
            v-model="channel.enabled"
            :aria-label="M.supportEnable(channel.name || String(index + 1))"
            :disabled="disabled"
          />
          <span class="text-admin-body">{{
            channel.enabled ? M.providerEnabled : M.providerDisabled
          }}</span>
        </div>
        <div class="flex items-center gap-2">
          <AdminButton
            variant="outline"
            :aria-label="M.supportMoveUp"
            :title="M.supportMoveUp"
            :disabled="disabled || index === 0"
            @click="move(index, -1)"
            ><NavArrowUp class="size-4"
          /></AdminButton>
          <AdminButton
            variant="outline"
            :aria-label="M.supportMoveDown"
            :title="M.supportMoveDown"
            :disabled="disabled || index === form.channels.length - 1"
            @click="move(index, 1)"
            ><NavArrowDown class="size-4"
          /></AdminButton>
          <AdminButton
            variant="destructive"
            :aria-label="M.supportRemove"
            :title="M.supportRemove"
            :disabled="disabled"
            @click="form.channels.splice(index, 1)"
            ><Trash class="size-4"
          /></AdminButton>
        </div>
      </div>
    </div>
    <AdminButton
      variant="outline"
      :class="adminActionButtonClass"
      :disabled="disabled || form.channels.length >= 20"
      @click="add"
      ><Plus class="size-4" />{{ M.supportAdd }}</AdminButton
    >
    <AdminAlert v-if="validationError" severity="error">{{ validationError }}</AdminAlert>
    <template #actions>
      <AdminButton
        :class="adminActionButtonClass"
        :disabled="disabled || !isDirty"
        :loading="saving"
        @click="save"
        >{{ M.save }}</AdminButton
      >
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        :disabled="disabled || !isDirty"
        @click="reset"
        >{{ M.reset }}</AdminButton
      >
    </template>
  </SettingEditorCard>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { NavArrowDown, NavArrowUp, Plus, Trash } from '@iconoir/vue'
import AdminAlert from '../shared/components/AdminAlert.vue'
import AdminButton from '../shared/components/AdminButton.vue'
import AdminFormField from '../shared/components/AdminFormField.vue'
import AdminInput from '../shared/components/AdminInput.vue'
import AdminSelect from '../shared/components/AdminSelect.vue'
import AdminSwitch from '../shared/components/AdminSwitch.vue'
import { useAdminConfirm } from '../shared/composables/useAdminConfirm'
import { confirmSettingsSave } from '../shared/lib/confirmSettingsSave'
import { adminActionButtonClass } from '../shared/lib/uiClasses'
import { newRequestId } from '../../../shared/api/contractKernel'
import { SUPPORT_CHANNELS_QUERY_KEY } from '../../../shared/composables/useSupportChannels'
import SettingEditorCard from './SettingEditorCard.vue'
import { SETTINGS_MESSAGES as M } from './pages/messages'
import { useSettingsLifecycle } from './useSettingsLifecycle'
import { useAdminSupportChannelsFeedback } from './useSettingsFeedback'
import {
  createAdminSupportChannelsResource,
  supportChannelKinds,
  supportChannelsQueryKey,
  type AdminSupportChannelsResource,
  type SupportChannelsSetting,
} from './supportChannelsResource'

const props = defineProps<{ resource?: AdminSupportChannelsResource }>()
const resource = props.resource ?? createAdminSupportChannelsResource()
const queryClient = useQueryClient()
const confirm = useAdminConfirm()
const query = useQuery({ queryKey: supportChannelsQueryKey, queryFn: () => resource.get() })
const mutation = useMutation({
  mutationFn: (input: { draft: SupportChannelsSetting; reason: string }) =>
    resource.update(input.draft, input.reason),
})
const loading = computed(() => query.isPending.value || query.isFetching.value)
const saving = computed(() => mutation.isPending.value)
let operatorReason = ''
const lifecycle = useSettingsLifecycle<SupportChannelsSetting>({
  defaultValue: { channels: [] },
  source: query.data,
  loadError: query.error,
  loading,
  saving,
  normalize: (setting) => ({
    channels: (setting?.channels ?? []).map((channel, index) => ({
      ...channel,
      name: channel.name.trim(),
      url: channel.url.trim(),
      sort_order: index,
    })),
  }),
  isSame: (left, right) => JSON.stringify(left) === JSON.stringify(right),
  validate: (draft) => {
    if (draft.channels.length > 20) return M.supportLimit
    for (const channel of draft.channels) {
      if (!channel.name.trim() || [...channel.name.trim()].length > 80) return M.supportNameInvalid
      try {
        if (/[\\\s]/u.test(channel.url.trim())) return M.supportUrlInvalid
        const url = new URL(channel.url.trim())
        if (
          !['https:', 'http:'].includes(url.protocol) ||
          url.username ||
          url.password ||
          channel.url.trim().length > 2048
        )
          return M.supportUrlInvalid
      } catch {
        return M.supportUrlInvalid
      }
    }
    return ''
  },
  save: (draft) => mutation.mutateAsync({ draft, reason: operatorReason }),
  synchronizeCache: async (saved) => {
    queryClient.setQueryData(supportChannelsQueryKey, saved)
    await queryClient.invalidateQueries({ queryKey: SUPPORT_CHANNELS_QUERY_KEY })
  },
  feedback: useAdminSupportChannelsFeedback(),
})
const { form, isDirty, reset, validationError } = lifecycle
const disabled = computed(() => loading.value || saving.value || Boolean(query.error.value))
const status = computed(() =>
  query.error.value
    ? M.providerLoadFailed
    : query.data.value
      ? M.supportCount(query.data.value.channels.filter((channel) => channel.enabled).length)
      : M.providerLoading,
)
const kindOptions = supportChannelKinds.map((kind) => ({
  value: kind,
  label: M.supportKinds[kind],
}))
function add() {
  form.value.channels.push({
    id: newRequestId(),
    name: '',
    kind: 'custom',
    url: '',
    enabled: true,
    sort_order: form.value.channels.length,
  })
}
function move(index: number, offset: number) {
  const target = index + offset
  if (target < 0 || target >= form.value.channels.length) return
  const [channel] = form.value.channels.splice(index, 1)
  if (channel) form.value.channels.splice(target, 0, channel)
}
async function save() {
  const reason = await confirmSettingsSave(confirm, {
    title: M.supportSave,
    message: M.supportSaveConfirm,
  })
  if (!reason) return
  operatorReason = reason
  await lifecycle.save()
}
</script>
