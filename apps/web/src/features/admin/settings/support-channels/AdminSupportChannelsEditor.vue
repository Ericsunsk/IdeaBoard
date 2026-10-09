<template>
  <div class="flex flex-col gap-3">
    <!-- 头部：板块名 + 副标题（图片库同款），添加按钮右上 -->
    <div class="flex flex-wrap items-center justify-between gap-3 pb-1">
      <div class="flex min-w-0 flex-col gap-0.5">
        <div class="flex items-center gap-2">
          <component
            :is="SETTINGS_SECTIONS.support.icon"
            class="size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
          <h3 class="text-admin-title-sm font-bold text-foreground">
            {{ SETTINGS_SECTIONS.support.title }}
          </h3>
        </div>
        <p class="text-admin-caption text-muted-foreground">
          {{ M.supportDescription }}
        </p>
      </div>
      <ActionButton :disabled="disabled || form.channels.length >= 20" @click="add"
        ><Plus class="size-4" />{{ M.supportAdd }}</ActionButton
      >
    </div>

    <!-- 全局标准表格 -->
    <div :class="adminTableCardClass">
      <AdminTable
        :value="form.channels"
        :loading="loading"
        data-key="id"
        scrollable
        :frame-class="adminTableFrameClass"
        :table-style="adminTableStyle"
        :class="adminTableClass"
        :table-aria-label="M.supportTitle"
      >
        <template #empty>
          <AdminEmptyState :title="M.supportEmpty" />
        </template>

        <!-- 渠道名称列 -->
        <AdminColumn
          field="name"
          :header="M.supportName"
          :style="{ width: '180px', minWidth: '150px' }"
        >
          <template #body="{ data }">
            <AdminInput
              :id="`support-name-${data.id}`"
              v-model="data.name"
              :aria-label="M.supportName"
              :disabled="disabled"
              :maxlength="80"
              size="small"
              class="w-full"
            />
          </template>
        </AdminColumn>

        <!-- 渠道类型列 -->
        <AdminColumn
          field="kind"
          :header="M.supportKind"
          :style="{ width: '150px', minWidth: '130px' }"
        >
          <template #body="{ data }">
            <AdminSelect
              :id="`support-kind-${data.id}`"
              v-model="data.kind"
              :options="kindOptions"
              :aria-label="M.supportKind"
              size="small"
              :disabled="disabled"
              class="w-full"
            />
          </template>
        </AdminColumn>

        <!-- 渠道链接列 -->
        <AdminColumn field="url" :header="M.supportUrl" :style="{ minWidth: '240px' }">
          <template #body="{ data }">
            <AdminInput
              :id="`support-url-${data.id}`"
              v-model="data.url"
              type="url"
              :aria-label="M.supportUrl"
              :disabled="disabled"
              :maxlength="2048"
              size="small"
              class="w-full font-mono"
            />
          </template>
        </AdminColumn>

        <!-- 操作列 -->
        <AdminColumn :header="M.actionsColumn" :style="{ width: '150px', minWidth: '140px' }">
          <template #body="{ index }">
            <div class="flex items-center gap-2">
              <AdminButton
                variant="outline"
                :aria-label="M.supportMoveUp"
                :title="M.supportMoveUp"
                :disabled="disabled || (index ?? 0) === 0"
                @click="move(index ?? 0, -1)"
                ><NavArrowUp class="size-4"
              /></AdminButton>
              <AdminButton
                variant="outline"
                :aria-label="M.supportMoveDown"
                :title="M.supportMoveDown"
                :disabled="disabled || (index ?? 0) === form.channels.length - 1"
                @click="move(index ?? 0, 1)"
                ><NavArrowDown class="size-4"
              /></AdminButton>
              <AdminButton
                variant="destructive"
                :aria-label="M.supportRemove"
                :title="M.supportRemove"
                :disabled="disabled"
                @click="form.channels.splice(index ?? 0, 1)"
                ><Trash class="size-4"
              /></AdminButton>
            </div>
          </template>
        </AdminColumn>

        <!-- 启用状态列 -->
        <AdminColumn field="enabled" :header="M.enableColumn" :style="{ width: '80px' }">
          <template #body="{ data, index }">
            <AdminSwitch
              v-model="data.enabled"
              :aria-label="M.supportEnable(data.name || String((index ?? 0) + 1))"
              :disabled="disabled"
            />
          </template>
        </AdminColumn>
      </AdminTable>
    </div>

    <AdminEditorActions
      :save-disabled="disabled || !isDirty"
      :reset-disabled="disabled || !isDirty"
      :saving="saving"
      @save="save"
      @reset="reset"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { NavArrowDown, NavArrowUp, Plus, Trash } from '@iconoir/vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import ActionButton from '../../../../shared/components/ActionButton.vue'
import AdminColumn from '../../shared/components/AdminColumn.vue'
import AdminEditorActions from '../../shared/components/AdminEditorActions.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import AdminSelect from '../../shared/components/AdminSelect.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import AdminTable from '../../shared/components/AdminTable.vue'
import {
  adminTableCardClass,
  adminTableClass,
  adminTableFrameClass,
  adminTableStyle,
} from '../../shared/lib/uiClasses'
import { newRequestId } from '../../../../shared/api/contractKernel'
import { SUPPORT_CHANNELS_QUERY_KEY } from '../../../../shared/composables/useSupportChannels'
import { SETTINGS_MESSAGES as M } from '../pages/messages'
import { SETTINGS_SECTIONS } from '../shared/sections'
import { useSettingsLifecycle } from '../shared/useSettingsLifecycle'
import { useAdminSupportChannelsFeedback } from '../shared/useSettingsFeedback'
import {
  createAdminSupportChannelsResource,
  supportChannelKinds,
  supportChannelsQueryKey,
  type AdminSupportChannelsResource,
  type SupportChannelsSetting,
} from './resource'

const props = defineProps<{ resource?: AdminSupportChannelsResource }>()
const resource = props.resource ?? createAdminSupportChannelsResource()
const queryClient = useQueryClient()
const query = useQuery({ queryKey: supportChannelsQueryKey, queryFn: () => resource.get() })
const mutation = useMutation({
  mutationFn: (draft: SupportChannelsSetting) => resource.update(draft, ''),
})
const loading = computed(() => query.isPending.value || query.isFetching.value)
const saving = computed(() => mutation.isPending.value)
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
  save: (draft) => mutation.mutateAsync(draft),
  synchronizeCache: async (saved) => {
    queryClient.setQueryData(supportChannelsQueryKey, saved)
    await queryClient.invalidateQueries({ queryKey: SUPPORT_CHANNELS_QUERY_KEY })
  },
  feedback: useAdminSupportChannelsFeedback(),
})
const { form, isDirty, reset } = lifecycle
const disabled = computed(() => loading.value || saving.value || Boolean(query.error.value))
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
  // Routine config write (backend ReasonOptional): save directly without
  // the confirm dialog; the audit records an empty reason.
  await lifecycle.save()
}
</script>
