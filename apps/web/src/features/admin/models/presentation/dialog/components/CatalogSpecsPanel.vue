<template>
  <div class="rounded-xl border border-border bg-card p-4 shadow-xs">
    <div class="mb-3 flex items-center justify-between border-b border-border pb-2.5">
      <h4 class="text-xs font-semibold text-foreground">{{ MODELS_MESSAGES.specsTitle }}</h4>
      <span class="text-admin-caption text-muted-foreground">{{
        MODELS_MESSAGES.specsSubtitle
      }}</span>
    </div>

    <div class="flex flex-col gap-3.5">
      <!-- 分辨率与托管类型 -->
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <!-- 分辨率 / 最大上下文 -->
        <div class="flex flex-col gap-1.5">
          <label
            class="text-admin-caption font-semibold text-foreground"
            for="model-resolution-input"
          >
            {{ MODELS_MESSAGES.specsResolutionLabel }}
          </label>
          <AdminInput
            id="model-resolution-input"
            :model-value="contextOrResolution"
            :placeholder="MODELS_MESSAGES.specsResolutionPlaceholder"
            @update:model-value="emit('update:contextOrResolution', String($event))"
          />
          <div class="flex gap-1 pt-0.5">
            <button
              v-for="res in ['1024×1024', '2048×2048', '4096×4096', '128K']"
              :key="res"
              type="button"
              class="rounded bg-muted px-1.5 py-0.5 text-admin-micro text-muted-foreground hover:bg-muted/80 cursor-pointer"
              @click="emit('update:contextOrResolution', res)"
            >
              {{ res }}
            </button>
          </div>
        </div>

        <!-- 托管类型 (Hosting) -->
        <div class="flex flex-col gap-1.5">
          <label class="text-admin-caption font-semibold text-foreground">{{
            MODELS_MESSAGES.specsHostingLabel
          }}</label>
          <div class="flex h-9 items-center rounded-lg border border-border bg-muted/40 p-1">
            <button
              type="button"
              class="flex-1 rounded-md py-1 text-xs font-medium transition-colors cursor-pointer text-center"
              :class="
                hosting === MODEL_HOSTING_CLOUDFLARE
                  ? 'bg-card text-emerald-700 shadow-xs font-semibold'
                  : 'text-muted-foreground hover:text-foreground'
              "
              @click="emit('update:hosting', MODEL_HOSTING_CLOUDFLARE)"
            >
              {{ MODELS_MESSAGES.specsHostingCloudflare }}
            </button>
            <button
              type="button"
              class="flex-1 rounded-md py-1 text-xs font-medium transition-colors cursor-pointer text-center"
              :class="
                hosting === MODEL_HOSTING_THIRD_PARTY
                  ? 'bg-card text-foreground shadow-xs font-semibold'
                  : 'text-muted-foreground hover:text-foreground'
              "
              @click="emit('update:hosting', MODEL_HOSTING_THIRD_PARTY)"
            >
              {{ MODELS_MESSAGES.specsHostingThirdParty }}
            </button>
          </div>
        </div>
      </div>

      <!-- 能力标签 -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label class="text-admin-caption font-semibold text-foreground" for="model-caps-input">
            {{ MODELS_MESSAGES.specsCapabilitiesLabel }}
          </label>
          <span class="text-admin-caption text-muted-foreground">{{
            MODELS_MESSAGES.specsCapabilitiesHint
          }}</span>
        </div>
        <AdminInput
          id="model-caps-input"
          :model-value="capabilitiesDraft"
          :placeholder="MODELS_MESSAGES.specsCapabilitiesPlaceholder"
          @update:model-value="emit('update:capabilitiesDraft', String($event))"
        />
        <div class="flex flex-wrap items-center gap-1 pt-0.5">
          <span class="text-admin-caption text-muted-foreground">{{
            MODELS_MESSAGES.specsQuickAdd
          }}</span>
          <button
            v-for="tag in MODELS_MESSAGES.specsPopularCapabilities"
            :key="tag"
            type="button"
            class="rounded-md border border-dashed border-border px-1.5 py-0.5 text-admin-caption font-medium text-muted-foreground hover:border-primary/40 hover:text-primary cursor-pointer"
            @click="addCapabilityTag(tag)"
          >
            + {{ tag }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import AdminInput from '../../../../shared/components/AdminInput.vue'
import { MODEL_HOSTING_CLOUDFLARE, MODEL_HOSTING_THIRD_PARTY } from '../../../domain/catalogData'
import { splitCapabilities } from '../../../domain/modelForm'
import { MODELS_MESSAGES } from '../../pages/messages'

const props = defineProps<{
  contextOrResolution: string
  hosting: string
  capabilitiesDraft: string
}>()

const emit = defineEmits<{
  'update:contextOrResolution': [value: string]
  'update:hosting': [value: string]
  'update:capabilitiesDraft': [value: string]
}>()

function addCapabilityTag(tag: string) {
  const current = splitCapabilities(props.capabilitiesDraft)
  if (!current.includes(tag)) {
    current.push(tag)
    emit('update:capabilitiesDraft', current.join(', '))
  }
}
</script>
