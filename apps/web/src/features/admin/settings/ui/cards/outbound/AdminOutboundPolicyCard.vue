<template>
  <SettingEditorCard
    :icon="SETTINGS_SECTIONS.outbound.icon"
    :title="SETTINGS_SECTIONS.outbound.title"
    :description="SETTINGS_MESSAGES.outboundDescription"
    :plain="props.plain"
    :class="props.class"
  >
    <template #header-actions>
      <AdminStatusPill v-if="!props.plain && statusLabel" :label="statusLabel" variant="neutral" />
    </template>
    <template #default>
      <div class="flex flex-col gap-5">
        <div v-if="loading && !setting" class="flex items-center gap-2" role="status">
          <AdminSpinner class="size-4" :aria-label="SETTINGS_MESSAGES.outboundLoading" />
          <span class="text-admin-body text-muted-foreground">{{
            SETTINGS_MESSAGES.outboundLoading
          }}</span>
        </div>

        <AdminFormField
          v-slot="{ describedbyId }"
          :label="SETTINGS_MESSAGES.outboundHostsLabel"
          for="outbound-policy-allow-hosts"
        >
          <AdminTextarea
            id="outbound-policy-allow-hosts"
            v-model="form.allow_hosts"
            :rows="5"
            class="w-full"
            :disabled="controlsDisabled"
            :aria-describedby="describedbyId"
          />
        </AdminFormField>
      </div>
    </template>

    <template #actions>
      <AdminEditorActions
        :save-disabled="controlsDisabled"
        :reset-disabled="controlsDisabled"
        :saving="saving"
        @save="save"
        @reset="reset"
      >
        <template #leading>
          <span class="text-admin-caption text-muted-foreground">
            {{ SETTINGS_MESSAGES.outboundHostsHint }}
          </span>
        </template>
      </AdminEditorActions>
    </template>
  </SettingEditorCard>
</template>

<script setup lang="ts">
import { computed, type HTMLAttributes } from 'vue'
import { AdminEditorActions } from '../../../../shared/index'
import { AdminStatusPill } from '../../../../shared/index'
import { AdminSpinner } from '../../../../shared/index'
import { AdminTextarea } from '../../../../shared/index'
import { AdminFormField } from '../../../../shared/index'
import SettingEditorCard from '../SettingEditorCard.vue'
import { SETTINGS_SECTIONS } from '../../pages/sections'
import { SETTINGS_MESSAGES } from '../../pages/messages'
import { useOutboundPolicy } from '../../../session/outbound/useOutboundPolicy'
import { useAdminOutboundPolicyFeedback } from '../../../session/useSettingsFeedback'

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class'] | undefined
    plain?: boolean
  }>(),
  { class: undefined, plain: false },
)

const feedback = useAdminOutboundPolicyFeedback()
const { error, form, loading, reset, save, saving, setting } = useOutboundPolicy({ feedback })

const controlsDisabled = computed(() => loading.value || saving.value || Boolean(error.value))
// 方案 A：加载中/加载失败不进 Status（只 toast），Status 仅反映已加载的真相源；
// 已额外放行是正常态（非默认但正常），走中性 secondary，不用 warn 黄。
const statusLabel = computed(() => {
  if (error.value) return undefined
  if (!setting.value) return undefined
  const count = setting.value.allow_hosts.length
  return count ? SETTINGS_MESSAGES.outboundExtraAllowed(count) : SETTINGS_MESSAGES.outboundNoneExtra
})
</script>
