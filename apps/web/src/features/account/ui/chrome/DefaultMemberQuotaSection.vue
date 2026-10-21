<template>
  <section class="flex min-w-0 flex-col gap-3" :aria-label="t('workspace.quota.defaultTitle')">
    <div
      v-if="section.isLoading.value"
      class="flex min-h-24 flex-col items-center justify-center gap-2"
      aria-live="polite"
    >
      <Spinner class="size-6 text-primary" />
      <span class="text-xs text-muted-foreground">
        {{ t('common.actions.loading') }}
      </span>
    </div>

    <LoadErrorPanel
      v-else-if="section.hasLoadError.value && !section.isNotConfigured.value"
      :message="t('workspace.quota.defaultLoadFailed')"
      :retry-label="t('common.actions.retry')"
      @retry="section.retry()"
    />

    <div v-else :class="accountCardPanelClass">
      <div class="flex items-center justify-between gap-2">
        <h3 class="m-0 text-admin-body font-semibold text-foreground">
          {{ t('workspace.quota.defaultTitle') }}
        </h3>
        <span v-if="section.policy.value" class="text-xs text-muted-foreground">
          {{ periodLabel(section.policy.value.period) }}
        </span>
      </div>

      <p v-if="section.policy.value" class="m-0 mt-2 text-sm text-foreground" aria-live="polite">
        <template v-if="section.policy.value.limitCredits != null">
          {{
            t('workspace.quota.defaultOf', {
              limit: section.policy.value.limitCredits,
            })
          }}
        </template>
        <template v-else>
          {{ t('workspace.quota.defaultUnlimited') }}
        </template>
      </p>
      <p
        v-else-if="section.isNotConfigured.value"
        class="m-0 mt-2 text-xs leading-normal text-destructive"
        role="alert"
      >
        {{ t('workspace.quota.defaultMissing') }}
      </p>

      <template v-if="section.canManage.value">
        <div class="mt-3 flex flex-col gap-2">
          <div
            class="flex flex-wrap items-center gap-2"
            :aria-label="t('workspace.quota.presetLabel')"
          >
            <Button
              v-for="preset in section.presets"
              :key="preset"
              type="button"
              variant="outline"
              size="sm"
              class="h-8 px-3"
              :is-disabled="section.isSaving.value"
              @click="section.pickPreset(preset)"
            >
              {{ t('workspace.quota.presetMonthly', { limit: preset }) }}
            </Button>
          </div>

          <label :for="periodSelectId" class="text-xs font-medium text-foreground">
            {{ t('workspace.quota.periodLabel') }}
          </label>
          <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The trigger carries its accessible name via :aria-label; the rule cannot see through the Select primitive. -->
          <Select
            :model-value="section.targetPeriod.value"
            :disabled="section.isSaving.value"
            @update:model-value="section.pickTargetPeriod($event)"
          >
            <SelectTrigger
              :id="periodSelectId"
              size="sm"
              class="w-full cursor-pointer gap-1.5"
              :aria-label="t('workspace.quota.periodLabel')"
            >
              <SelectValue>{{ periodLabel(section.targetPeriod.value) }}</SelectValue>
              <SelectIcon>
                <NavArrowDown class="size-3.5 shrink-0" aria-hidden="true" />
              </SelectIcon>
            </SelectTrigger>
            <SelectContent position="popper" align="start" :side-offset="appOverlayMenuSideOffset">
              <SelectViewport>
                <SelectItem value="daily">
                  <SelectItemText>{{ t('workspace.quota.periodDaily') }}</SelectItemText>
                </SelectItem>
                <SelectItem value="weekly">
                  <SelectItemText>{{ t('workspace.quota.periodWeekly') }}</SelectItemText>
                </SelectItem>
                <SelectItem value="monthly">
                  <SelectItemText>{{ t('workspace.quota.periodMonthly') }}</SelectItemText>
                </SelectItem>
                <SelectItem value="fixed">
                  <SelectItemText>{{ t('workspace.quota.periodFixed') }}</SelectItemText>
                </SelectItem>
                <SelectItem value="unlimited">
                  <SelectItemText>{{ t('workspace.quota.periodUnlimited') }}</SelectItemText>
                </SelectItem>
              </SelectViewport>
            </SelectContent>
          </Select>

          <template v-if="section.targetPeriod.value !== 'unlimited'">
            <label :for="limitInputId" class="text-xs font-medium text-foreground">
              {{ t('workspace.quota.limitLabel') }}
            </label>
            <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The limit input carries its accessible name via aria-label; the rule cannot see through the Input primitive. -->
            <Input
              :id="limitInputId"
              v-model="section.limitDraft.value"
              size="sm"
              class="h-9 min-w-0 flex-1 bg-background text-xs"
              :disabled="section.isSaving.value"
              :aria-label="t('workspace.quota.limitLabel')"
              inputmode="numeric"
              @keydown.enter="section.saveDefault()"
            />
          </template>
          <div class="flex shrink-0 gap-2">
            <Button
              type="button"
              size="sm"
              class="h-9 px-4"
              :is-loading="section.isSaving.value"
              :is-disabled="section.isSaving.value"
              @click="section.saveDefault()"
            >
              {{ section.isSaving.value ? t('workspace.quota.saving') : t('workspace.quota.save') }}
            </Button>
          </div>
        </div>
        <p
          v-if="section.saveError.value"
          class="m-0 text-xs leading-normal text-destructive"
          role="alert"
        >
          {{ section.saveError.value }}
        </p>
        <p class="m-0 mt-2 text-xs text-muted-foreground">
          {{ t('workspace.quota.defaultAffectsNewOnly') }}
        </p>
      </template>
      <p v-else class="m-0 mt-2 text-xs text-muted-foreground">
        {{ t('workspace.quota.defaultManagerOnly') }}
      </p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { NavArrowDown } from '@iconoir/vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectIcon,
  SelectItem,
  SelectItemText,
  SelectTrigger,
  SelectValue,
  SelectViewport,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import LoadErrorPanel from '../../../../core/components/LoadErrorPanel.vue'
import { appOverlayMenuSideOffset } from '../../../../theme/appOverlayMenu'
import { useDefaultMemberQuotaSection } from '../../session/quota/useDefaultMemberQuotaSection'
import { quotaPeriodLabelKey, type MemberQuotaPeriod } from '../../session/quota/resource'
import { accountCardPanelClass } from '../shared/ui'

const props = withDefaults(
  defineProps<{
    workspaceId: string
    consumptionScope?: string | null | undefined
    isExpanded?: boolean | undefined
  }>(),
  {
    consumptionScope: null,
    isExpanded: true,
  },
)

const { t } = useI18n()

const section = useDefaultMemberQuotaSection({
  workspaceId: () => props.workspaceId,
  consumptionScope: () => props.consumptionScope,
  isExpanded: () => props.isExpanded,
})

const periodSelectId = `default-quota-period-${props.workspaceId}`
const limitInputId = `default-quota-limit-${props.workspaceId}`

function periodLabel(period: MemberQuotaPeriod): string {
  return t(quotaPeriodLabelKey(period))
}
</script>
